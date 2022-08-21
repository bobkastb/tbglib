package pool_ar

import (
	"fmt"
	"os"
	"sync/atomic"
	"tbglib/tools"
	"time"
)

type CLog struct {} 
var currlog CLog;
func (l *CLog) LPrintf(level int,format string, v ...interface{}) {
	f:= os.Stdout; if (level<0) {f=os.Stderr};
	fmt.Fprintf( f , format ,  v ... )
} 
type CLogStream struct {} 
func (s*CLogStream) LogErrorEvent( e error ){
	ebp,ok := e.(*tools.ErrorWithCode);
	if (ok) {
		if (ebp.Code==Err_Breaked_Fetch) { return }
		f:= os.Stdout; if (ebp.Level<0) {f=os.Stderr};
		fmt.Fprintf( f , "error: %d %s \n" , ebp.Level , ebp.Txt )
	}

};
var currlogstream CLogStream;

type Recourse_ProcPlug struct {
	name string;
	id int32;
	state int;
	finstate int;
	//pool_state int32;
	//owner_pool IPoolOfResource;
	pool IPoolOfResource; 
	pool_data *TResourcePoolRec;
}	
func (prc *Recourse_ProcPlug) IsFinalizing() int {
	 return prc.state;};
func (prc *Recourse_ProcPlug) UnwantedFromPoll(){ prc.Stop(); }; 

func (prc *Recourse_ProcPlug) CommonToPool( pool IPoolOfResource , pd*TResourcePoolRec) error { 
	prc.pool = pool;
	prc.pool_data= pd; return nil;
 }
func (prc *Recourse_ProcPlug) GetPoolVars() *TResourcePoolRec { return prc.pool_data;};
func (prc *Recourse_ProcPlug) GetOwnedPool() IPoolOfResource { return prc.pool; }
//func (prc *Proc) SetOwnerPool( pool IPoolOfResource){ prc.owner_pool = pool; }

func (prc *Recourse_ProcPlug) IsTerminate() bool {
	return prc.finstate>=2;
}

func (prc *Recourse_ProcPlug) GetTerminateChan() <- chan int {
	return nil;
}
func (prc *Recourse_ProcPlug) TestWorkState() error{
	return nil;
}

func (prc *Recourse_ProcPlug) AfterKill() error {
	if prc.finstate>=3 { return nil; }
	prc.finstate=3; prc.state = Erest_Finalized;
	if (prc.pool!=nil) { 
		prc.pool.Resource_Finalized(prc);}	
	return nil
}
func (prc *Recourse_ProcPlug) Kill_prep() error {	
	if prc.finstate>=2 { return nil; }
	prc.finstate=2; prc.state = Erest_Finalizing;
	return prc.pool.Resource_Finalizing(prc);
}	
func (prc *Recourse_ProcPlug) Kill() error {	
	if prc.finstate>=2 { return nil; }
	finstate := prc.finstate; prc.finstate=2; prc.state = Erest_Finalizing;
	if (finstate==0 && prc.pool!=nil) { prc.pool.Resource_Finalizing(prc);}
	if (prc.pool==nil) { return nil;}
	//prc.pool.Resource_Finalized(prc);	
	go func () { time.Sleep(10*time.Millisecond); prc.AfterKill(); }();
	return nil;};

func (prc *Recourse_ProcPlug) Stop() error {	
	if prc.finstate>=1 { return nil; }
	finstate := prc.finstate; prc.finstate=1;	prc.state = Erest_Finalizing;
	if (finstate==0 && prc.pool!=nil) { prc.pool.Resource_Finalizing(prc);}
	if (prc.pool!=nil) { prc.pool.Resource_Finalizing(prc);}
	go func () { time.Sleep(10*time.Millisecond); prc.Kill(); }();
	return nil;};



	
var cntr_proc int32=0;
func NewProcPlug( pool IPoolOfResource ) (*Recourse_ProcPlug, error) {
	atomic.AddInt32(&cntr_proc,1);
	res:= &Recourse_ProcPlug{
		pool : pool,
		id : cntr_proc,
		name:fmt.Sprintf("res-%d",cntr_proc),
		pool_data: nil,
	};
	return res,nil;
}



