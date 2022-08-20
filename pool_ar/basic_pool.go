package pool_ar

import (
	//"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"tbglib/tools"
	"time"
)

// Интерфейс ресурса, которым управляет пул. Внутри только одна функция.
type ISingleResource interface{

	// UnwantedFromPoll: Соощение ресурсу о том что его не хотят более использовать. Это не страшно. Просто был лишний "возврат" ресурса (Release): Err_ReleasePullFull и 
	//Нужно закрыть ресурс. Ресурс попытался вернуться а pool уже полный. Ресурс остается Erespool_Free , то есть его нельзя вернуть(Release), и необходимо закрывать.
	UnwantedFromPoll(); 

}

/* Пул ресурсов. 
Пул создается функцией NewPool
Ресурс должен иметь интерфейс ISingleResource. Ресурс может неожиданно завершится.
Ресурс должен сообщать пулу о том что он завершился или находится в стадии завершения.
Примеры ресурсов: процесс ОС, горутина .... 
Если ресурс завершился или завершается , пул считает что такой ресурс  нельзя использовать и не выдает его.
Выдача ресурса (или захват) производится функцией Fetch.
Захваченный ресурс должен быть возвращен пулу (функцией Release) вне зависимости от состояния ресурса.
Если пул инициализирован NewPool() без указания функции для создания ресурс , то пользователь получит nil после fetch  и должен создавать ресурс самостоятельно, при этом возвращать ресурс в пул обязательно.
Простое правило: каждому Fetch нужен Release, без исключений
Концепция:
	1) Забираем ресурс Fetch-м , если ошибка считаем что ресрс нам НЕ дали (закругляемся)
	2) Если ресурс==nil, значит от нас ждут что мы сами сделаем новый ресурс (мы не сказли пулу как делать новые ресурсы)
	3) Отдаем ресурс обязательно (Release)! Лучше сразу
	Но,если ресурс планируется уничтожить, лучше сразу начать процесс уничтожения и сказать об этом пулу (resource_stopping)
	4) В случае уничтожения вызывать Resource_Finalized, иначе пул будет держать ресурс (IPoolOfResource) в памяти
	5) Не стоит отдавать чужие ресурсы (те которые присоеденялись к другим пулам )
*/
type IPoolOfResource interface{
	// Перенаправить сообщения об ошибках
	SetErrorStream(elog tools.ILogErrorEvent);
	// Перенаправить лог-сообщения  пула
	SetLogger(log tools.ILevelLogger);
	
	//Захват ресурса из пула с ожиданием. interrupt_chans - массив каналов прерывателей ожидания. 
	//Любое событие (данные,закрнытие) в одном из каналов приведет к прерыванию ожидания. 
	// При прерывании ожидания возвращается ошибка Err_Breaked_Fetch
	Fetch( interrupt_chans  ... interface{} ) (ISingleResource, error)
	//Возврат ресурса в пул
	Release(proc ISingleResource) error
	// Закрыть пул. Если wait==true, то будет ожидание завершения захватов (fetch) во всех горутинах
	Close( wait bool)

	GetLimit( ) int // максимальное количество ресурсов
	GetCntFree( ) int  // количество свободных ресурсов]
	
	Resource_Finalizing( r ISingleResource ) error; // ресурс сигнализирует о начале завершение 
	Resource_Finalized( r ISingleResource ) error; // ресурс сигнализирует о завершении
}


const (
	Err_PoolClosed   		= 1
	Err_Breaked_Fetch   	= 2
	Err_Unregistred_Fetch 	= 3
	Err_Repeat_Registry 	= 4

	Err_Unregistred_Release = 5
	Err_Repeat_Release   	= 6
	Err_PullFull_Release   	= 7
	Err_PoolLimitExceeded 	= 8

	Err_Repeat_Finalized 	= 9 
	Err_Unregistred_Finalized = 10
	Err_Repeat_Finalizing 	= 11

	Err_PoolInternal		= 12
	Err_PushInvalidPool 	= 13
//
//Err_InvalidUseCounter = 14
//Err_ReleaseInvalidPool   = 4
	
	Err_MaxErrorNumber = 13
)


type ErrorBasicPool struct { 	tools.ErrorWithCode; pool IPoolOfResource; res ISingleResource; }

const ( Erest_Nothing =1 ; Erest_Work=2 ; Erest_Finalizing=3 ;Erest_Finalized=4 )
type Trest_status int32;
const ( Erespool_Nothing =0 ; // pool незнаком с теми кто это хранит
	Erespool_Free =1 ; // ресурс в пуле и НЕ занят
	Erespool_Busy =2 ; // ресурс НЕ в пуле и Занят
 )
 type Trespool_status int32;
 const ( 
	Epooloperation_NotInit =0 // pool считает что этот ресурс он еще не видел
	Epooloperation_NotUse =1 // pool считает что этот ресурс нельзя использовать
	Epooloperation_Use  =2  //pool использует этот ресурс
 )
 type Tpooloperation_state int32;

// Тип функции для создания нового ресурса из пула 
type fCreateResFromPool	func(pool IPoolOfResource ) (ISingleResource,error);

// Статистика работы пула ресусов. Активно используется при тестировании. Может быть использована в логировании
type Pool_statictic_rec struct {
	all_created int64; // Полное количество созданных ресурсов
	all_closed int64 // Полное количество уничтоженых ресурсов
	all_unwanted int32 // Полное количество нежелательных ресурсов
	pushed, 		// Количество возвращенных ресурсов
	pushed_break ,  // Количество возвращенных ресурсов в стадии завершения
	pushed_nil int32 // Количество возвращенных ресурсов после уничтожения
	popcnt int32 // Количество захватов живых ресурсов
	popnil,  // Количество захватов при которых надо создавать новый ресурс
	pop_Finalized, // Количество захватов уничтоженных ресурсов
	pop_Finalizing , // Количество захватов ресурсов в стадии уничтожения
	pop_interrupt int32 // Количество прерываний процедуры захвата 
	created   int32 // Количество активных ресурсов
	maxcreated int32 // Максимальное количество активных ресурсов
	maplen int32 // количество ресурсов в хранилище (storage)
	errors int32 // общее количество ошибок
	errors_a [Err_MaxErrorNumber+1]int32 // Количество ошибок каждого типа.

} 

// Структура для хранения данных ресурса
type TResourcePoolRec struct {
	// индикатор того что ресурс занят (захвачен)
	busy_status int32;
	// ресурс вернулся в состоянии завершения
	f_push_on_finalize bool
	// количество ошибок связанных с ресурсом
	pool_error int32
	owner IPoolOfResource
	// состояние ресурса - ( Erest_Nothing =1 ; Erest_Work=2 ; Erest_Finalizing=3 ;Erest_Finalized=4 )
	work_status Trest_status;
	// Количество захватов (Fetch) и освобождений (Release) данного ресурса
	stat_op_cntr [2]int32

//inpool_cntr int32
//	isFinalized int32 // 0 work , 1 - Finalized
//pool_operating_status int32;//Tpooloperation_state;  Epooloperation_***
}   


type Basic_Pool struct {
	idname string // Идентификатор пула. 
	stat Pool_statictic_rec; // статистика пула
	active bool // Пул активен
	countInQue int32 //Очередь за фетчем. Нужен для того чтобы Пул мог ждать пока все от него отвалятся.
	pool      chan ISingleResource // канал - по сути очередь ресурсов
	mut *sync.Mutex // собственный мьютекс
	new_resource fCreateResFromPool; // функция создания новго ресурса
	logger tools.ILevelLogger;  // интерфейс лога
	errlog tools.ILogErrorEvent; // Сигнальный поток ошибок
	// storage: Вместо sync.Map -> tools.ISyncMap . По умолчанию создается map без мьюткса. Но можно задать внешний , например общий storage для всех пулов (с мьютексом).
	// Важно что интерфейс не зависит от фактического использования мьютекса в storage.
	// По сути это map[ISingleResource]*TResourcePoolRec 
	storage tools.ISyncMap;  
	errmask_IgnLog int64   // битовая маска игнорирования ошибок. Каждой ошибке соотвествует свой номер бита
	errmask_UseConst int64

	//dbg_storage  map[ISingleResource]*TResourcePoolRec;
	//poolLimit int // справочная переменная, только чтение
	//que_destroyed  chan ISingleResource
	//interrupt_stub chan interface{}
}

// Перенаправить сообщения об ошибках
func (p*Basic_Pool) SetErrorStream(elog tools.ILogErrorEvent){  p.errlog = elog; };

// Перенаправить лог-сообщения  пула
func (p*Basic_Pool) SetLogger(log tools.ILevelLogger){ p.logger = log;};

// Нового пула.  limit - максимальное количество активных ресурсов. newr - функция создания нового ресурса.
func NewPool( limit int , newr fCreateResFromPool ) *Basic_Pool {
	if (limit<=0) { limit = 5;}
	
	p:= &Basic_Pool{
		//poolLimit: runtime.NumCPU() * 2,
		stat:Pool_statictic_rec {
			all_created:0 ,all_closed :0 , created:0,
			all_unwanted: 0,
		},
		idname:"",
		new_resource : newr,
		storage : tools.NewMapGenNSync(false), //sync.Map{},
		//storage : &sync.Map{},
		active:true,
		pool:make(chan ISingleResource, limit ),
		mut : &sync.Mutex{},
		errmask_IgnLog :(1<<Err_Repeat_Finalized)|(1<<Err_Repeat_Finalizing),
		errmask_UseConst: (1<<Err_Repeat_Finalized)|(1<<Err_Repeat_Finalizing),
		//dbg_storage : make( map[ISingleResource]*TResourcePoolRec ),
	}

	if p.logger==nil { p.logger=tools.GetDefaultLogger(); }
	for i:=0;i<limit;i++ {
		p.pool <- nil;
	}
	p.active = true
	return p;
}

// Возвращает количество свободных ресурсов в пуле
func (p *Basic_Pool) GetCntFree( ) int{ 	return len(p.pool); }

// Возвращает максимальное количество ресурсов в пуле
func (p *Basic_Pool) GetLimit( ) int{ 	return cap(p.pool); }

 
func (p *Basic_Pool) newResourse() (ISingleResource, error) {
	if (p.new_resource==nil) { return nil,nil; }
	prc,e := p.new_resource( p ) ;
	if (e!=nil) {  

	} else { _,e=p.RegNewResourse( prc ); }
	return prc,e;
}

// pool_lock_ctx создан для того чтобы не писать сообщения в логер, до тех пор пока код назодтся внутри мьютекса
type pool_lock_ctx struct {
	err *ErrorBasicPool;
	pool *Basic_Pool;
	islock bool 
}
func (lctx *pool_lock_ctx) Lock() { lctx.islock=true; lctx.pool.mut.Lock(); }

func (lctx *pool_lock_ctx) Unlock() { lctx.islock=false; lctx.pool.mut.Unlock();
	if (lctx.err!=nil) {lctx.pool.pushError2Log(lctx.err); lctx.err=nil;}
 }

func _makeErrorBasicPool( level int , errcode int , text string ) *ErrorBasicPool {
	ewc := tools.ErrorWithCode{Level:level ,Code: errcode , Txt:text } 
	return &ErrorBasicPool{ ErrorWithCode: ewc };
}
var _constPoolErrMaps = map[int]*ErrorBasicPool{
	Err_Repeat_Finalized:_makeErrorBasicPool(tools.WarningLevel,Err_Repeat_Finalized," Repeat finalized" ) ,
	Err_Repeat_Finalizing:_makeErrorBasicPool(tools.WarningLevel,Err_Repeat_Finalizing," Repeat finalizing" ) ,
}

func (lctx *pool_lock_ctx) MakeErrorAndMsg(level int , code int,  errstr string) *ErrorBasicPool { 
	lctx.err= lctx.pool.makeErrorOnly( level , code , errstr )
	e:=lctx.err;
	if !lctx.islock { lctx.pool.pushError2Log(lctx.err); lctx.err=nil; }
	return e;
 }
func (lctx *pool_lock_ctx) MakeError(code int, errstr string) *ErrorBasicPool { return lctx.MakeErrorAndMsg(tools.VerboseLevel,code,errstr); }


func (p *Basic_Pool) makeErrorOnly( level int , code int,  errstr string) *ErrorBasicPool {
	if 0==(p.errmask_IgnLog & (1<<code)){ 		
		atomic.AddInt32( &p.stat.errors ,1 )
		if (code<len(p.stat.errors_a)) { atomic.AddInt32( &p.stat.errors_a[code] ,1 ) }
	}
	if 0!=(p.errmask_UseConst & (1<<code)){ return _constPoolErrMaps[code]	}

	return &ErrorBasicPool{ 
		ErrorWithCode:tools.ErrorWithCode{Txt:errstr, Code: code ,  Level:level}, 
		pool:p,
	}	
}	
func (p *Basic_Pool) pushError2Log( err*ErrorBasicPool)  { 
	if 0!=(p.errmask_IgnLog & (1<<err.Code)){  return;	}
	if (p.logger!=nil) { p.logger.LPrintf( err.Level , "%s:%s\n",p.idname, err.Txt ); }
	if (p.errlog!=nil) { p.errlog.LogErrorEvent( err ) }

}
func (p *Basic_Pool) _MakeErrorAndMsg( level int , code int,  errstr string) *ErrorBasicPool { 
	e:= p.makeErrorOnly( level , code , errstr )
	p.pushError2Log(e);
	return e;
}
func (p *Basic_Pool) _MakeError_Verb( code int, errstr string) *ErrorBasicPool { return p._MakeErrorAndMsg(tools.VerboseLevel,code,errstr); }	
func (p *Basic_Pool) _MakeError(code int, errstr string) *ErrorBasicPool { return p._MakeErrorAndMsg(tools.VerboseLevel,code,errstr); }


//--------------

func (p *Basic_Pool) RegNewResourse( res ISingleResource) (*TResourcePoolRec,error) {
	lctx := pool_lock_ctx{ pool:p}; lctx.Lock();defer lctx.Unlock(); 
	//p.mut.Lock(); defer p.mut.Unlock(); 

	pr:=&TResourcePoolRec{
		busy_status: Erespool_Busy,
		pool_error:0,
		owner : p,
		work_status: Erest_Work,
		f_push_on_finalize:false,
	};
	rv,load := p.storage.LoadOrStore(res,pr);
	atomic.AddInt32( &p.stat.maplen , 1)

	rvp := rv.(*TResourcePoolRec);
	if (load) { 
		if (rvp.owner!=p) {	return  rvp,lctx.MakeError(Err_PushInvalidPool, "Reourse in other pool");	}	
		return rvp,lctx.MakeError(Err_Repeat_Registry, "Resourse already n pool");
	}else {
		atomic.AddInt64(&p.stat.all_created ,1)
		_created:=atomic.AddInt32(&p.stat.created,1)
		_maxcreated:=p.stat.maxcreated
		if (_created>_maxcreated) {
			atomic.CompareAndSwapInt32(&p.stat.maxcreated ,_maxcreated, _created );
			if (_created>int32(p.GetLimit())) {
				lctx.MakeErrorAndMsg( tools.WarningLevel , Err_PoolLimitExceeded, fmt.Sprint("Pool Limit Exceeded! ",_created,"\n") );
			}	
		}	
	}	
	return rvp,nil;
	
}

func (p *Basic_Pool) getResourceVars(res ISingleResource) *TResourcePoolRec{
	if (res==nil) { return nil; }
	r,load := p.storage.Load(res);
	if load { return r.(*TResourcePoolRec) } 
	return nil;
}


// ресурс сигнализирует о начале завершение 
func (p *Basic_Pool) Resource_Finalizing( res ISingleResource )error { 
	lctx := pool_lock_ctx{ pool:p}; lctx.Lock();defer lctx.Unlock(); 
	//p.mut.Lock(); defer p.mut.Unlock(); 
	rv := p.getResourceVars(res);
	if (rv==nil) {return lctx.MakeError( Err_Unregistred_Finalized ,"Finalizing: Unregistred resource" );}

	if  !atomic.CompareAndSwapInt32( (*int32)(&rv.work_status) , Erest_Work , Erest_Finalizing ) {
		return lctx.MakeError( Err_Repeat_Finalizing ,"Repeat Resource_Finalizing" );
	}
	return nil;
 };

func (p *Basic_Pool) delete_resourse( res ISingleResource , lctx * pool_lock_ctx )  {
	_,ok := p.storage.LoadAndDelete( res );
	if (ok) {
		//resvars := r.(*TResourcePoolRec)
		//if (resvars.stat_op_cntr[0]!=resvars.stat_op_cntr[1]) {	p._MakeError(Err_PoolInternal,"ResourseFinalized: InternalError!")}
		atomic.AddInt32( &p.stat.maplen , -1)
	}
}

const eContinueWait = true
func (p *Basic_Pool) FetchHandler( resp * ISingleResource , vbreak bool ) ( bool , error ) {
	lctx := pool_lock_ctx{ pool:p}; lctx.Lock();defer lctx.Unlock(); 
	//p.mut.Lock(); defer p.mut.Unlock(); 
	
	res:=*resp;
	atomic.AddInt32( &p.stat.popcnt , 1 )

	if res!=nil {
		rv:= p.getResourceVars(res);
		if (rv!=nil) {
			
			rv.stat_op_cntr[0]++;
			if Erespool_Free!=atomic.SwapInt32( &rv.busy_status , Erespool_Busy) {
				return false,lctx.MakeError(Err_PoolInternal,"Fetch: Internal Error!");
			};
			if (rv.work_status== Erest_Finalized ) {
				p.delete_resourse(res, &lctx); 
				*resp=nil
				atomic.AddInt32( &p.stat.pop_Finalized , 1 )
			} else if rv.work_status==Erest_Finalizing { // продолжаем сидеть в чтении каналов. Нельзя возвращать res просящему.
				atomic.AddInt32( &p.stat.pop_Finalizing , 1 )
				rv.f_push_on_finalize = true;
				return  eContinueWait ,nil; // Это единственный продолжатель цикла
			} 
		} else {
			//TODO:? Что делать???
			return false,lctx.MakeError(Err_Unregistred_Fetch,"Fetch: Ресурс не зарегистрирован!");
		}	
	} else {
		atomic.AddInt32( &p.stat.popnil , 1 )
	}


	return false,nil
}	
//func (p *Basic_Pool) tryBackToPool( )
func (p *Basic_Pool) user_interrupt_err(  ) error { 
	atomic.AddInt32(&p.stat.pop_interrupt,1)
	return p._MakeErrorAndMsg(tools.VerboseLevel,Err_Breaked_Fetch,"Interrupted");  // прерывание пользователя
}	

//Захват ресурса из пула с ожиданием. interrupt_chans - массив каналов прерывателей ожидания. Любое событие (данные,закрнытие) в одном из каналов приведет к прерыванию ожидания
func (p *Basic_Pool) Fetch( interrupt_chans  ... any ) (ISingleResource, error) {
	var res ISingleResource = nil;
	var err error=nil; 
	//indexOfPool:=0
	indexOfPool:= len(interrupt_chans);
	var local_chans [10]any ; var chans []any;
	if (indexOfPool==0) { // не будем без необходимости засорять хип
		chans = tools.EcoCat_AnyArray( local_chans[0:], []any{p.pool} , interrupt_chans... );
	} else { chans = tools.EcoCat_AnyArray( local_chans[0:],interrupt_chans,p.pool ); }

	for (p.active) {
		// Прерывания в приоритетет. Правильно конечно проверять это после Read_chans, но тогда придется восстанавливать пул (класть туда то что сняли)
		if tools.IsAnySignalInChans( interrupt_chans... ) { 
			return nil,p.user_interrupt_err(); }

		res = nil;vbreak:=false;cont := false;
		atomic.AddInt32(&p.countInQue,1) // сообщаем пулу что мы зашли в очередь
		selindex , value , ok := tools.Read_chans( chans...  )
		atomic.AddInt32(&p.countInQue,-1) // сообщаем пулу что мы больше не в очереди

		if (selindex==indexOfPool ) {
			if (!ok) { break;} // Pool closed
			if !value.IsNil() {res = value.Interface().(ISingleResource); }
		} else { 
			return nil,p.user_interrupt_err();  // прерывание пользователя
		}
		cont,err = p.FetchHandler( &res,vbreak );
		if (cont==eContinueWait) { continue }
		if (res==nil && err==nil) { 
			res,_=p.newResourse(); } 
		return res,err
	}
	return nil,p._MakeErrorAndMsg(tools.VerboseLevel,Err_PoolClosed,"Fetch:Pool closed");  // Пул закрывается и ничего никому не раздает
}

// ресурс сигнализирует о завершении
func (p *Basic_Pool) Resource_Finalized( res ISingleResource ) error{ 
	// проверка что нет повторов вызова Resource_Finalized
	//p.mut.Lock(); 	defer p.mut.Unlock(); 
	lctx := pool_lock_ctx{ pool:p}; lctx.Lock();defer lctx.Unlock(); 


	resvars := p.getResourceVars(res);
	if (resvars==nil) { 
		return lctx.MakeError(Err_Unregistred_Finalized,"Unregistred resource on Finalized signal") }
	if atomic.SwapInt32( (*int32)(&resvars.work_status) , Erest_Finalized )==Erest_Finalized {
		return lctx.MakeError(Err_Repeat_Finalized,"Repeat Finalized signal")
	}
	p.stat.created--;
	p.stat.all_closed++;
	if !p.active { return lctx.MakeError(Err_PoolClosed,"ResourseFinalized: Pool closed!") }
	if ( !resvars.f_push_on_finalize ) { return nil }
	if Erespool_Busy != atomic.SwapInt32( &resvars.busy_status , Erespool_Free ){
		return nil; } // Ресурс уже в пуле
	select {
	case p.pool <- nil:
		p.delete_resourse(res,&lctx);
		p.stat.pushed_nil++;
	default: 
		resvars.busy_status = Erespool_Busy;
		return lctx.MakeError(Err_PullFull_Release,"que_destroyed is full! Resource signed as invalid")
	}
	return nil
};

type _ErrPullFull = bool
func (p *Basic_Pool) push( res ISingleResource ) *ErrorBasicPool  {
	if tools.IsNil(res) {	
		return nil; }
	lctx := pool_lock_ctx{ pool:p}; lctx.Lock();defer lctx.Unlock(); 
	//p.mut.Lock(); defer p.mut.Unlock(); 

	resvars := p.getResourceVars(res)
	if (resvars==nil) { 
		return lctx.MakeError(Err_Unregistred_Release,"Resource unregistred"); }; 
	if Erespool_Busy!=atomic.SwapInt32( &resvars.busy_status , Erespool_Free ) {
		return lctx.MakeError(Err_Repeat_Release,"Repeat release!"); }
	if (resvars.f_push_on_finalize) { 
		return lctx.MakeError(Err_Repeat_Release,"Repeat release!"); }
	
	if ( resvars.work_status== Erest_Finalized) {
		p.delete_resourse(res,&lctx); 
		atomic.AddInt32( &p.stat.pushed_nil , 1 )
		res = nil
	} else if (resvars.work_status==Erest_Finalizing )	{ 
		// если ресурс в стадии ликвидации, то его не нужно возвращать в пул, в пул не пойдет ничего
		resvars.busy_status = Erespool_Busy;
		resvars.f_push_on_finalize = true;
		atomic.AddInt32( &p.stat.pushed_break , 1 )
		return nil; 
	} else {
		resvars.stat_op_cntr[1]++;
	} 

	atomic.AddInt32( &p.stat.pushed , 1 )

	if !p.active { return lctx.MakeError(Err_PoolClosed,"Release: Pool closed!") }
	//if ( spec_st >=Erest_Stopping || res.IsFinalizing()>=Erest_Stopping ) { 	res=nil; }  
	select {
	case p.pool <- res:
	default: // ктото все таки прорвался "лишний" раз в Release. Это плохой признак  такого быть не может
		// В любом случае, нам такой ресурс не нужен, надо сообщить о нем куда следует
		atomic.AddInt32(&resvars.pool_error,1)
		atomic.AddInt32(&p.stat.all_unwanted,1)
		resvars.busy_status = Erespool_Busy;
		return lctx.MakeError(Err_PullFull_Release,"Pull is full! Resource signed as invalid")
	}
	return nil;
};	


//Возврат ресурса в пул
func (p *Basic_Pool) Release(res ISingleResource) error {
	e := p.push(res);
	if (e!=nil && e.Code==Err_PullFull_Release ) {  res.UnwantedFromPoll(); }
	if (e==nil) {return nil}
	return e
}


// Закрыть пул. Если wait==true, то будет ожидание завершения захватов (fetch) во всех горутинах
func (p *Basic_Pool) Close( wait bool){
	func () {
		p.mut.Lock(); 
		defer p.mut.Unlock(); 
		p.active = false;
		close(p.pool ) // прерываем всех кто ждет в фетче 
	}()
	if (wait) {for ( p.countInQue > 0 ) {
		// опционально ждем пока все покинут очередь за фетчем
		time.Sleep( 100*time.Millisecond ); 
	}}	
	
}


