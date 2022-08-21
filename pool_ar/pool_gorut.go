package pool_ar

import (
	"sync"
	"sync/atomic"
	"tbglib/tools"
	"time"
	// "time"
)

/*
Задачу высоконагруженной гомогенной обработки может решить по другому.
Можно не ограничивать напрямую количество ресурсов, а ограничить количество горутин занимающихся обработкой.
При этом каждой горутине соответствует один и тот же набор типов активных ресурсов
Каждая горутина ожидает заданий из канала (chan) , результаты выполнения задания направляются в другой канал (канал результатов)

Особенности :
	1) Ресурсы не должны создаваться в горутине до появления задания. после завершения задания ресурс не уничтожается, и будет использован повторно с новым заданием
	2) Каждое задание имеет ограничение по времени выполнения (таймер)
	3) Принудительное завершение задания приводит к необходимости завершения "ресурса", так по сути именно он отнимает время
	4) Создание ресурса может быть затраным по времени, поэтому таймер задания должен включаться после приведения ресурса в состояние готовности
	5) Результаты выполнения задания, не должны поедать много памяти...

func  NewPoolOfWorker( limit int, taskdef ITaskDefinition , run bool  )
func (p*PoolOfWorker) Close( wait bool )
func (p*PoolOfWorker) Start(  )

*/
type PoolOfWorker struct {
	RTaskDefinitionsData
	mu *sync.Mutex
	//tasks_in chan ITask;
	//tasks_out chan ITaskResult;
	Sig_stop chan int; // сигнал для остановки всех или нескольких ProcWorker
	Sig_fin chan int
	taskdef ITaskDefinition; // фабрика ресурсов
	//Task_default_timeout time.Duration
	//Resourse_create_tumeout time.Duration
	//Resourse_close_tumeout time.Duration
	max_workers int32
	count_workers int32
	active int32

}

type InterruptChan = <- chan int;

type ProcWorker struct {
	//WSig_stop chan int; // сигнал для остановки данного экземпляра
	pool  *PoolOfWorker; // пул - владелец
	proc ISingleResource; // текущий ресурс
	state int
	err error
	Timer_timeout *tools.TimeoutSig;
	interrupt_chans [2]InterruptChan;
	interrupt_index int
	//NewTimeoutSig(
}

const ( wkst_Nothing=0; wkst_Normal=1; wkst_WaitSendResult=2; wkst_WaitCloseResourse=4; wkst_Finalize=100;)
// основная процедура ProcWorker
func (pw*ProcWorker) wProcess(){  
	
	pw.state=wkst_Normal
	//if !pw.pool.workerAttach(pw) { pw.state=100; pw.pool.workerDettach(pw); return }
	for (pw.state==wkst_Normal) {
	select {
		case task := <- pw.pool.Tasks_in : {
			if !pw.pool.taskdef.OnPopTaskFromQue(task) { 
				tr := pw.pool.taskdef.HandleDOS( task, nil )
				pw.send_result( task ,  tr , nil ); 
				continue;
			}
			//tres,e := 
			pw.runTask( task )
		};
		//case <-pw.WSig_stop : pw.onInterrupt(); 
		case <-pw.pool.Sig_stop: pw.onInterrupt(); 
	}} 
	pw.state=wkst_Finalize;
	pw.close_resource(  )
	pw.pool.workerDettach(pw)
};

// Вызыается обработчиком задачи, если сработал один из прерывателей
func (pw*ProcWorker) onInterruptSignal( chnumber int ){  
	pw.interrupt_index=chnumber;
}
// Возвращает два прерывателя [ стоп сигнал пула (Sig_stop) , таймер таймаута задачи ]
func (pw*ProcWorker) GetInterrupts( )[2]InterruptChan{  
	return pw.interrupt_chans;
}
func (pw*ProcWorker) runTask( task ITask  ) (ITaskResult,error){  
	var tres ITaskResult=nil
	task_to := pw.pool.GetTimeoutForTask(task);
	e := pw.test_resource_state(pw.proc);
	if (e!=nil ) { // ресурс нестабилен, закроем его 
		pw.close_resource(  ); e=nil; }	
	if ( tools.IsNil( pw.proc ) ) {
		// ресурс не готов, создаем новый
		pw.proc,e = pw.make_resource();
		task_to += pw.pool.GetTimeoutForCreateProcess( "" );
		if (pw.err!=nil) {
			// ошибка при создании ресурса, закрываем его
			pw.close_resource(  )
		}
	}
	if (e!=nil) {
		// ошибка ресурса. Генерируем ответ для задачи, не запуская задачу (не на чем)
		tres = pw.makeErrorForTask( task , e )
		pw.send_result( task ,  tres , e ); 
	} else {	 
		pw.Timer_timeout = tools.NewTimeoutSig( task_to )
		pw.interrupt_index=-1;
		pw.interrupt_chans[0]= pw.pool.Sig_stop; pw.interrupt_chans[1]= pw.Timer_timeout.C;
		tres,e = pw.pool.taskdef.ProcHandler( pw , task  );
		pw.send_result( task ,  tres , e ); 
		switch (pw.interrupt_index) {
			case 0: pw.onInterrupt() 
			case 1: pw.close_resource(); //timeout
		};

	}	
	return tres,e;
}	

// Завершение процесса
func (pw*ProcWorker) close_resource(  ){  
	proc := pw.proc;
	if (tools.IsNil(proc)) { return  }
	if pw.pool.Allow_kill_resource { proc.Kill(); return; }
	proc.Stop()
	ech := proc.GetTerminateChan()
	if tools.IsNil(ech) { ech = tools.GetClosedIntChan(); } //TODO: ничего не ждем - только Kill, а надо бы обождать
	oldstate := pw.state;
	to := tools.NewTimeoutSig( pw.pool.Resourse_close_tumeout );
	pw.state=wkst_WaitCloseResourse;
	select {
		case <- ech: 
		case <- to.C: 
		//case <-pw.WSig_stop :  pw.onInterrupt(); break;
		case <-pw.pool.Sig_stop: pw.onInterrupt();break;
	}	
	if (!proc.IsTerminate()) { proc.Kill()}
	if (pw.state!=wkst_Finalize) { pw.state = oldstate }
}	

func (pw*ProcWorker) make_resource(  ) (ISingleResource,error) {  
	return  pw.pool.taskdef.NewResource( nil ) 
}	
func (pw*ProcWorker) test_resource_state( proc ISingleResource ) (error) {  
	if !tools.IsNil(proc) {
		return proc.TestWorkState()
	}
	return nil
}	
func (pw*ProcWorker) makeErrorForTask( task ITask , e error ) ITaskResult {  
	return pw.pool.taskdef.HandleDOS( task , e)
}

func (pw*ProcWorker) onInterrupt(  ){  
	switch (pw.state) {
		case wkst_Normal:  //TODO: нас прервали при ожидании задания
		case wkst_WaitSendResult:  //TODO: нас прервали при отправке результата
	}
	pw.state=wkst_Finalize;
}

func (pw*ProcWorker) send_result( t ITask , tres ITaskResult , err error ){  
	oldstate := pw.state;
	pw.state= wkst_WaitSendResult;
	pw.err = err;
	select {
	case pw.pool.Tasks_out <- tres:	pw.state=oldstate; 
	//case <-pw.WSig_stop : pw.onInterrupt();break;
	case <-pw.pool.Sig_stop: pw.onInterrupt();
	}	
}

func (p*PoolOfWorker) newProcWorker() *ProcWorker {
	retv := &ProcWorker{
		//WSig_stop : make(chan int), 
		pool:p,
		proc:nil,
		state:wkst_Nothing,
		err:nil,
	}
	return retv;
}

func (p*PoolOfWorker) GetTimeoutForTask( t ITask) time.Duration {  return p.Task_default_timeout;  }
func (p*PoolOfWorker) GetTimeoutForCreateProcess( proctype any ) time.Duration {  return p.Resourse_create_tumeout;  }


func (p*PoolOfWorker) reStart( with_f func()  )  {  
	p.mu.Lock(); defer 	p.mu.Unlock();
	with_f();
	if (p.active==0) {return};
	cnt := p.count_workers - p.max_workers;
	for  ;cnt>0 && p.active!=0;cnt-- { 
		if !tools.Try_push_toIntChan(p.Sig_stop, 0) { break };
	}
	for p.count_workers<p.max_workers {
		p.count_workers++;
		pw := p.newProcWorker();
		go pw.wProcess();
	}
}

	
func (p*PoolOfWorker) Start(  )  {  
	prev:=atomic.SwapInt32(&p.active,1)
	if (prev!=0) {return};
	p.reStart( func(){} );
}
// прицепиться к пулу
func (p*PoolOfWorker) workerAttach( pw*ProcWorker  ) bool {  return p.active!=0;  }
// Отцепиться от пула
func (p*PoolOfWorker) workerDettach( pw*ProcWorker  ) { 
	p.reStart( func(){p.count_workers--;} );
	tools.Try_push_toIntChan( p.Sig_fin , 1 )
}


func (p*PoolOfWorker) Close( wait bool )  {  
	prev:=atomic.SwapInt32(&p.active,0)
	if (prev==0) {return};
	p.reStart( func() { close(p.Sig_stop)  } );
	for (wait && p.count_workers>0) {
		<-p.Sig_fin;
	}
}	

func  NewPoolOfWorker( limit int, taskdef ITaskDefinition , run bool  ) *PoolOfWorker {
	retv := &PoolOfWorker{
		RTaskDefinitionsData: taskdef.GetTaskDefinitionsData() ,
		mu :&sync.Mutex{},
		Sig_stop : make(chan int,16),
		Sig_fin : make(chan int),
		taskdef: taskdef,
		//tasks_in : _tin,
		//tasks_out: _tres ,
		//Task_default_timeout:1000*time.Millisecond,
		//Resourse_create_tumeout:1000*time.Millisecond,
		//Resourse_close_tumeout:1000*time.Millisecond,
		max_workers : int32( limit ),
		count_workers:0,
		active:0 } 
	if (run) { retv.Start() }	
	return retv;	
}


