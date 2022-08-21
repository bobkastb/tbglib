package tools

import (
	"bytes"
	"io"
	"reflect"
	"sync"
	"sync/atomic"
	"time"
)

/*
------- Tools -------
type IBytesTransformer interface

func IsNil( v any ) bool
func EcoCat_AnyArray( localarr []any, leftarr []any, rightarr ... any )[]any
func Read_chans( icha ...any) (int,reflect.Value,bool)
func TryRead_chans( icha ...any) (int,reflect.Value,bool)
func IsAnySignalInChans(icha ...any) bool
func TryWriteChan( achan any , value any ) bool

---------- Timeout
func NewTimeoutSig( d time.Duration ) *TimeoutSig
type TimeoutSig struct { C <- chan int; }
func (t*TimeoutSig) Finish() bool
func (t*TimeoutSig) Stop() bool

*/

// IIBytesTransformer - дубль интерфейса Transformer из "golang.org/x/text/encoding/unicode" , для того чтобы не привязывать модуль в внешним зависимостям
type IBytesTransformer interface {
	Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error)
	Reset()
}

// Возвращает количество символов "c" в буффере
func Find_count_ofchar( b []byte , c byte ) int {
	res:=0;
	for (len(b)>0) {
		r:=bytes.IndexByte( b , c);
		if (r<0) { return res; }
		res++
		b =b[r+1:]
	}
	return res;
}

// TimeoutSig: Одно поле для пользования - C (<- chan int;). Закрывает этот канал по таймауту. Все кто читает канал - отваливаются.
// NewTimeoutSig(длительность) - создает , Finish - закрывает канал ,Stop - прерывает выполнение
// Основное отличие от стандарного таймера - канал закрывается, а не просто заполняется. То есть повторная попытка чтения канала после срабатывания таймера, не приведет к ожиданию
type TimeoutSig struct {
	C <- chan int; // Читая этот канал мы ждем таймера. Когда время кончилось этот канал будет закрыт

	// no access
	fstopped bool;
	finished int32
	c chan int; 
	timer *time.Timer;
}
// Создает новый таймер TimeoutSig
func NewTimeoutSig( d time.Duration ) *TimeoutSig{
	t:=&TimeoutSig{ fstopped:false , finished:0 ,c:nil,C:nil };
	if (d!=0) {
		t.c = make( chan int); 
		t.C = t.c;	
		t.timer=time.AfterFunc( d , func() { t.Finish() } )
	}	
	return t
}

// Принудительный переход TimeoutSig в состояние "таймаут"
func (t*TimeoutSig) Finish() bool {
	if 1==atomic.AddInt32( &t.finished , 1 ) {
		t.Stop();
		close(t.c);
	}	
	return true;
}	

// Останов работы таймера. Канал не закрывается
func (t*TimeoutSig) Stop() bool {
	t.fstopped = true
	return t.timer.Stop();
}	

// Проверка any на равенство nil. Простое v==nil не срабатывает, если v типизиванный указатель 
func IsNil( v any ) bool {
	return v==nil || ( reflect.ValueOf(v).Kind() == reflect.Ptr && reflect.ValueOf(v).IsNil())
}	

// IfElse Operator <условие>? <если истина> : <если ложь>
func IfElse( cond bool , if_val any , else_val any )any{
	if (cond) { return if_val;}; return else_val;
}

// Экономный вариант append. Сделан для того чтобы лишний раз не засорять кучу. Локальный массив имееь шанс отстаться локальным.
// localarr - результирующий массив .  localarr = append(leftarr,rightarr)
func EcoCat_AnyArray( localarr []any, leftarr []any, rightarr ... any )[]any{
	szleft :=len(leftarr); sz := szleft + len(rightarr);
	res:=localarr;
	if (len(localarr)< sz) { res= make([]any,sz) }
	for i,v := range leftarr { res[i]=v }
	for i,v := range rightarr { res[i+szleft]=v }
	return res[0:sz]

}

// I2ChanValue: возвращает reflect.Value если ach  это Chan
func I2ChanValue( ach any ) (reflect.Value,bool) {
	//var iZ interface{} = nil
	vc := reflect.ValueOf(ach);
	ok := vc.IsValid() && vc.Type().Kind()==reflect.Chan 
	if !ok { vc = reflect.ValueOf(any(nil)); } 
	return vc,ok
}

func fillSelectCaseArray( branches []reflect.SelectCase , icha []any){
	//var iZ interface{} = nil
	for i,ic := range icha{
		vc,_ := I2ChanValue( ic )
		sc := reflect.SelectCase{ Dir:reflect.SelectRecv , Chan: vc  };
		branches[i] = sc;
	}
}

func fillOrNewSelectCaseArray( asz int, local []reflect.SelectCase , icha []any) []reflect.SelectCase{
	sz := asz+len(icha)
	if len(local) <= sz {
		res := make( []reflect.SelectCase , asz+len(icha) );
		fillSelectCaseArray( res , icha  );
		return res;
	} else { 
		fillSelectCaseArray( local , icha  ); 
		return local[0:sz]  
	};
}


// Read_chans : Ожидание чтения одного из нескольких каналов. Равносильна оператору select для всех каналов. Разница в том что набор каналов динамический, то есть задается в параметре
// icha - набор каналов. Возвращает номер канала из которого прошло чтение, прочитанное значение, признак закрытия канала
func Read_chans( icha ...any) (int,reflect.Value,bool){
	var local_br [10]reflect.SelectCase;
	branches:= fillOrNewSelectCaseArray( 0 , local_br[0:] , icha )
	selIndex, vRecv, sentBeforeClosed := reflect.Select(branches);
	return selIndex, vRecv, sentBeforeClosed;
}

//var zero_reflect_value=reflect.ValueOf(0)

// Попытка чтения из набора каналов. Если попытка не удалась, ожидания нет и возвращаемый индекс находится за пределами массива icha	
// Эквивалентно конструкции select + default
func TryRead_chans( icha ...any) (int,reflect.Value,bool){
	def_index := len(icha)
	if (def_index==0) {return def_index,reflect.ValueOf(0),false;}
	var local_br [10]reflect.SelectCase;
	branches:= fillOrNewSelectCaseArray( 1 , local_br[0:] , icha )
	branches[def_index] =  reflect.SelectCase{ Dir:reflect.SelectDefault  } ;
	selIndex, vRecv, sentBeforeClosed := reflect.Select(branches);
	return selIndex, vRecv, sentBeforeClosed;
}

// Проверяет сигнал в любом из каналов. Считается что данные в каналах не существенны.
// Возвращает true если хотяюы в одном из каналов были данные или канал закрыт. 
func IsAnySignalInChans(icha ...any) bool {
	if (len(icha)==0) {return false}
	selIndex,_,_:= TryRead_chans( icha... )
	return selIndex<len(icha);
}

			
func TryWriteChan( achan any , value any ) bool{
	vc,ok := I2ChanValue( achan )
	if (!ok) { return false; }
	rv , ok := value.(reflect.Value); if (!ok) { rv = reflect.ValueOf(value) }
	local_br := [2]reflect.SelectCase{
		{ Dir:reflect.SelectSend , Send: rv ,Chan: vc  }, 
		{ Dir:reflect.SelectDefault  }, 
	}  
	selIndex, _, _ := reflect.Select(local_br[0:]);
	return (selIndex==0);
}

//ISyncMap придуман как замена sync.Map. Его можно создавать без мьютекса.
type ISyncMap interface{
	LoadOrStore(key, value any) (actual any, loaded bool) 
	Load(key any) (value any, ok bool)
	LoadAndDelete(key any) (value any, loaded bool)
	Range( func(key, value any) bool )
}

type MapGenNSync struct{
	dmap map[any]any;
	mu *sync.Mutex
}

// Создает MapGenNSync с интерфейсом sync.Map . 
func NewMapGenNSync( mutexuse bool ) *MapGenNSync {
	var mut *sync.Mutex=nil
	if (mutexuse) { mut= &sync.Mutex{} }
	return &MapGenNSync{ dmap: map[any]any{}, mu:mut	};
}
func ( mp * MapGenNSync ) Lock() { if (mp.mu!=nil) { mp.mu.Lock() } }
func ( mp * MapGenNSync ) Unlock() { if (mp.mu!=nil) { mp.mu.Unlock() } }
func ( mp * MapGenNSync ) Load(key any) (value any, ok bool) {
	mp.Lock(); defer mp.Unlock()
	value,ok = mp.dmap[key]; return;
}
func ( mp * MapGenNSync ) LoadAndDelete(key any) (value any, loaded bool){
	mp.Lock(); defer mp.Unlock()
	value,loaded = mp.dmap[key]; 
	if (loaded) { delete(mp.dmap, key) }
	return 
}
func ( mp * MapGenNSync ) LoadOrStore(key, value any) (actual any, loaded bool) {
	mp.Lock(); defer mp.Unlock()
	actual,loaded = mp.dmap[key]
	if (!loaded) {
		mp.dmap[key] = value;
		actual = value
	}
	return;
}

func ( mp * MapGenNSync ) Range( uf func (key, value any) bool ) {
	type kvt struct { k,v any };
	var lst []kvt;
	func () {
		mp.Lock(); defer mp.Unlock()
		lst = make( []kvt ,  len(mp.dmap) );
		i:=0; for k, v := range mp.dmap { lst[i] = kvt{ k,v };i++ }
	}();

	for _,v := range lst { uf( v.k,v.v)	}
}


// cBytesTransformer: Этот декодер использует конечный размер буффера при кодировке символов, и не сваливается в ожидание
type cBytesTransformer struct {	
	resin_cnt int
	in_membuff []byte
	out_membuff []byte
	trans IBytesTransformer;
	lasterr error
	wrstream io.Writer;
}


func NewcBytesTrasformer( trans IBytesTransformer , sz int  ) *cBytesTransformer {
	if (sz==0) { sz = 256; }
	return &cBytesTransformer{ trans:trans ,
		resin_cnt:0,in_membuff:make([]byte,sz),out_membuff:make([]byte,sz),lasterr:nil,
	 }
}
func (t *cBytesTransformer) Write( b []byte ) (int,error) { 
	if (t.wrstream==nil) { return 0,io.EOF };//TODO:
	n:= t.Write2Writer( b , t.wrstream);
	return n,nil
}	

func (t *cBytesTransformer) Write2Writer( b []byte , wr io.Writer ) int { 
	f:= func( outd []byte) (int,error) { 
		return wr.Write(outd); }
	return t.Write2Func( b , f )
}

func (t *cBytesTransformer) Write2Func(b []byte , ff func([]byte) (int,error) ) (int)  { 
	fwc := 0; 
	for (len(b)>0) {
		wc :=  t.Write2Buff( b ); 
		b = b[wc:];
		//if (wc) 
		fwc += wc
		t.Flush(ff)
	}	
	t.Flush(ff)
	return fwc;
}
func (t *cBytesTransformer) Flush( wrF func([]byte) (int,error) ) (int)  { 
	cnt:=0;
	for { ready:= t.ReadAll(); 
		cnt+= len(ready);
		if (len(ready)==0) { break }
		wrF(ready) 
	}
	return cnt;
}

func (t *cBytesTransformer) DoTransform() int  {
	nDst,nSrc,err := t.trans.Transform( t.out_membuff , t.in_membuff[:t.resin_cnt] , false  );
	t.lasterr = err;
	remcnt := t.resin_cnt - nSrc;
	if remcnt>0 && nSrc>0 { // shift left
		copy( t.in_membuff , t.in_membuff[nSrc:t.resin_cnt] )
	}
	t.resin_cnt -= nSrc;
	return nDst
}
func (t *cBytesTransformer) Write2Buff(b []byte) (int)  { 
	cntcap := copy( t.in_membuff[t.resin_cnt:] , b );
	t.resin_cnt += cntcap;
	return cntcap;
}	
func (t *cBytesTransformer) ReadAll() []byte  { 
	lenout :=t.DoTransform()
	return t.out_membuff[:lenout]; 
}


func Try_push_toIntChan( ch chan int , val int ) bool {
	select { case ch <- 0: return true; default: return false }	
}

var defClosedIntChan <-chan int;
var defInfiniteIntChan <-chan int = make(chan int);

func GetClosedIntChan() <-chan int {
	return defClosedIntChan; 
}
func GetInfiniteIntChan() <-chan int {
	return defInfiniteIntChan; 
}