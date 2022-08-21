package pool_ar

import (
	"fmt"
	"sync/atomic"
	"tbglib/tools"
	"testing"
	"time"
)

type tTask1 struct {
	data string
	active bool
}

type tTaskResult1 struct {
	task *tTask1;
	data string
	err error
}
type tTaskDefinition1_Stat struct {
	cntResource int32;
	cntHandlers	int32;
	cntHandlersErrRes int32;
	cntHandlersBreak int32;
	cntResetTask int32;
}
type tTaskDefinition1 struct {
	data RTaskDefinitionsData;
	stat tTaskDefinition1_Stat;
	//proc
}
func (td *tTaskDefinition1) NewResource( pool IPoolOfResource ) (ISingleResource,error) {
	atomic.AddInt32( &td.stat.cntResource , 1 )
	return NewProcPlug( pool );
}
func (td *tTaskDefinition1) ProcHandler( own *ProcWorker , task ITask  ) (ITaskResult,error) {
	atomic.AddInt32( &td.stat.cntHandlers , 1 )
	time.Sleep( 10*time.Millisecond );
	torg :=  task.(*tTask1);
	return &tTaskResult1{ task: torg , data: torg.data , err:nil  }, nil
}
var errBreakTask = &tools.ErrorWithCode{ Txt:"Break Task"};
func (td *tTaskDefinition1) HandleDOS( task ITask, e error) ITaskResult {
	torg :=  task.(*tTask1);
	if (e==nil && !torg.active) {	
		atomic.AddInt32( &td.stat.cntHandlersBreak , 1 )
		e=errBreakTask;
	} else { atomic.AddInt32( &td.stat.cntHandlersErrRes , 1 ) }
	return &tTaskResult1{ task: torg , data: torg.data , err:e  }
}
func (td *tTaskDefinition1) OnPopTaskFromQue( task ITask) bool {
	torg :=  task.(*tTask1);
	if (!torg.active) { atomic.AddInt32( &td.stat.cntResetTask , 1 ) }
	return torg.active;
}
func (td *tTaskDefinition1) GetTaskDefinitionsData() RTaskDefinitionsData {
	return td.data;
};

func new_tTaskDefinition1() * tTaskDefinition1 {
	retv := &tTaskDefinition1{ 
		data: RTaskDefinitionsData{
		Task_default_timeout : 1000 *time.Millisecond,
		Resourse_create_tumeout : 1000 *time.Millisecond,
		Resourse_close_tumeout : 1000 *time.Millisecond,
		Allow_kill_resource  :true,
		Tasks_in : make( chan ITask , 100),
		Tasks_out : make( chan ITaskResult , 100),
		}}
	return retv;
};

type testTskInfo struct {
	task ITask; res ITaskResult; 
}

func testpgr_1(t *testing.T){
	//PoolOfWorker
	td := new_tTaskDefinition1()
	pool:= NewPoolOfWorker( 10 , td , true )
	t.Log("testpgr_1:: start")

	tlist := make( map[string]*testTskInfo);

	cnttask := 200; cntResetTask := int(float32(cnttask)*0.1);
	for i :=0; i<cnttask;i++ { 
		task := &tTask1{ data:fmt.Sprintf("id:%d",i) , active: i>=cntResetTask };
		tlist[task.data] = &testTskInfo{ task:task, res:nil }; 
	}	
	go func() {	for _,tsk := range tlist { 	td.data.Tasks_in <- tsk.task; } } ()


	for i :=0; i<cnttask;i++ {
		tri:= <- td.data.Tasks_out;
		tr := tri.(*tTaskResult1);
		tle,ok:= tlist[tr.task.data ]
		if !ok { Fail(t); continue; }
		if (tle.res!=nil) {
			Fail(t); continue; }
		tle.res = tr;
	}
	t.Log("testpgr_1:: ending")
	pool.Close(true);
	 
	if ( td.stat.cntResource != 10 ) { Fail(t); }
	if ( td.stat.cntHandlers != int32(cnttask - cntResetTask) )  { Fail(t); }
	if ( td.stat.cntHandlersBreak != int32(cntResetTask) )  { Fail(t); }
	if ( td.stat.cntResetTask != int32(cntResetTask) )  { Fail(t); }
	if ( td.stat.cntHandlersErrRes != 0 )  { Fail(t); }
	//fmt.Println(td.stat)
	t.Log("testpgr_1:: ending")
}


func Test_Poolgr(t *testing.T){
	testpgr_1(t)
}
