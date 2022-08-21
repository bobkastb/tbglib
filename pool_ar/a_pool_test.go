package pool_ar

import (
	"fmt"
	"runtime"
	"sync/atomic"
	"tbglib/tools"
	"testing"
	"time"
)


func Fail(t *testing.T) {
	funcName,file,line,ok := runtime.Caller(1);
	//t.Errorf("error at %s func %s line %d %v",file,funcName,line,ok);
	t.Errorf("error at %s func (%d) line %d %v",file,funcName,line,ok);
}

func fetch_proc( p IPoolOfResource , ex ... any ) (*Recourse_ProcPlug,*ErrorBasicPool){
	r,e := p.Fetch(ex...);
	err := (*ErrorBasicPool)(nil);
	if (e!=nil) { err = e.(*ErrorBasicPool) }
	if (r==nil) { 
		return nil,err; }
	return r.(*Recourse_ProcPlug),err;
}

func Pool_NewProc(pool_ IPoolOfResource) (ISingleResource,error) { return NewProcPlug(pool_); }

type tst_gen_stat struct {
	cnt_timeout int
}

type th_ctx struct {
	thid int
	exitchan chan int
	worktime time.Duration
	do_timeout bool
	do_kill bool
	do_kill_afterR bool
	gstat *tst_gen_stat
} 

func longtst_thproc(p IPoolOfResource , ctx th_ctx ) error{
	//worktime:=30*time.Millisecond;
	defer func() { ctx.exitchan <- 0 }()
	//println("start rut",tn);
	var timeout_dur time.Duration=0; 
	if (ctx.do_timeout) {
		timeout_dur =ctx.worktime/10;}
	timeout:=tools.NewTimeoutSig( timeout_dur );
	/*intc := make(chan int);
	if ( ctx.do_timeout ) { 	go func() { time.Sleep( ctx.worktime/10 ); close(intc);	 }()	
	} else { intc=nil }*/
	r,e := fetch_proc(p,timeout.C);
	defer func() { 
		p.Release(r) 
		if ctx.do_kill_afterR && r!=nil { r.Kill() }
	 }()
	if (e!=nil) {
		fmt.Printf("r:%d error:%s res:%#v\n", ctx.thid ,e,r); 
		return e;}
	time.Sleep( ctx.worktime/2 )	
	time.Sleep( ctx.worktime/2 )	
	//println("Release rut",tn);
	
	if ( ctx.do_kill ) {
		r.Kill();
	}; 
	//println("end rut",tn);
	return nil;

}


func Pool_long_test(){
	//fnp := func(pool_ IPoolOfResource) (ISingleResource,error) { return NewProc(pool_); }
	pool := NewPool(10 ,  Pool_NewProc );
	pool.SetErrorStream( &currlogstream )
	gstat :=&tst_gen_stat{}
	//pool.SetLogger(&currlog);
	
	test_prc :=func (cntproc int) {
		fmt.Println("---------Start test",cntproc)
		gstat =&tst_gen_stat{}
		exchan := make( chan int , 10 );
		for tid:= 0;tid<cntproc;tid++ {
			ctx := th_ctx{
				thid :tid , exitchan : exchan, worktime: 30* time.Millisecond,
				gstat : gstat,
				do_timeout : tid % 20 ==0 ,
				do_kill : tid % 5==0,
				//do_kill_afterR : tid % 30==0,
				do_kill_afterR : tid % 11==0,
			}
			if (ctx.do_kill_afterR) { ctx.do_kill=false; }
			go longtst_thproc( pool , ctx );
		}
		//time.Sleep( 10*time.Millisecond )	
		//fmt.Println("---------Wait exit all routine")
		for i:= 0;i<cntproc;i++ {
			<-exchan;
		}
		fmt.Println("---------end test",cntproc)
	}	
	//test_prc(30)
	test_prc(1000);
	test_prc(100);

	fmt.Println("---------Pool closing")
	pool.Close(true);
	//time.Sleep( 3*time.Second )	
	fmt.Printf( "%#v\n maplen:%d" ,  pool.stat , pool.GetCntFree()  )

}




func test1(t *testing.T){
	//fnp := func(pool_ IPoolOfResource) (ISingleResource,error) { return NewProc(pool_); }
	pool := NewPool(1 ,  Pool_NewProc );
	pool.SetErrorStream( &currlogstream )
	st := &pool.stat; 

	prc,err := fetch_proc(pool); // Kill без Prep до Release 
	if (err!=nil) {Fail(t);}
	e:=prc.AfterKill(); if (e!=nil) {Fail(t);}
	e= pool.Release(prc); 
	if (st.all_closed!=1 || st.pushed!=1 || st.pushed_nil!=1) { Fail(t);}

	prc1,err := fetch_proc(pool); // Kill без Prep во время Release
	if (err!=nil || prc1==nil || prc1.id==prc.id) { Fail(t); }
	e=pool.Release(prc1); if (e!=nil) {Fail(t);} 
	e=prc1.AfterKill();   if (e!=nil) {Fail(t);}

	prc,err = fetch_proc(pool); // Не должны схватить предыдцщий
	if (err!=nil || prc==nil || prc1.id==prc.id) { Fail(t); }
	e=pool.Release(prc); if (e!=nil) {t.Fail()} 

	prc1,err = fetch_proc(pool); // Должны схватить предыдущий + Prep до Release, Kill после
	if (err!=nil || prc==nil || prc1.id!=prc.id) { Fail(t); }
	e=prc1.Kill_prep();   if (e!=nil) {t.Fail()} 
	e=pool.Release(prc1); if (e!=nil) {t.Fail()}  // еще не вернулся
	if (pool.GetCntFree()!=0 || st.pushed_nil!=1 )	{t.Fail()} 
	e=prc1.AfterKill();	  if (e!=nil) {t.Fail()} //вот теперь вернулся
	if (pool.GetCntFree()!=1 || st.errors!=0 || st.pushed_nil!=2 )	{t.Fail()} 
	//fmt.Println(pool);

	*st = Pool_statictic_rec{}
	prc1,err = fetch_proc(pool); // pop_finalised + Prep после Release до Fetch, Kill во время Fetch
	if (err!=nil || prc==nil || st.all_created!=1 ) { Fail(t); }
	if nil!=pool.Release(prc1) {t.Fail()} 
	e=prc1.Kill_prep();   if (e!=nil) {t.Fail()} 
	go func(){ time.Sleep(10*time.Millisecond); prc1.AfterKill()}()// kill with delay
	prc,err=fetch_proc(pool); if (err!=nil || prc==nil  ) { Fail(t); }
	if st.pop_Finalizing!=1 || st.all_created!=2 {Fail(t);} 
	if nil!=pool.Release(prc) {t.Fail()} 

	// timeout.  пул пуст, и должен сработать таймаут 
	*st = Pool_statictic_rec{};
	ch := make(chan int,1); 
	prc,_ = fetch_proc(pool,ch);
	ch <- 1
	prc1,err = fetch_proc(pool,ch);  
	if (err==nil || err.Code!= Err_Breaked_Fetch ) {Fail(t);} 
	pool.Release(prc);pool.Release(prc1);
	if (pool.GetCntFree()!=1 || st.errors!= st.errors_a[Err_Breaked_Fetch]) {Fail(t);} 

	// timeout.  пул НЕ пуст, но должен сработать таймаут 
	*st = Pool_statictic_rec{};
	ch<-1 ; 
	prc,err = fetch_proc(pool,ch);
	if (err==nil || err.Code!= Err_Breaked_Fetch || pool.GetCntFree()!=1 || st.errors!= st.errors_a[Err_Breaked_Fetch]) {Fail(t);} 
	//fmt.Println("--",err,pool.GetCntFree(),len(ch),"")

	pool.Release(prc);
	/*close(ch);
	prc1,err = fetch_proc(pool,ch);  // пул не пуст, но должен сработать таймаут 
	if (err==nil || err.code!= Err_Breaked_Fetch ) {Fail(t);} 
	*/
	


}
func test_Close(t *testing.T){
	pool := NewPool(1 ,  Pool_NewProc );
	//pool.SetErrorStream( &currlogstream )
	st := &pool.stat; var fincnt int32=0
	cnt := 10; entered:=int32(0)
	fetch_proc(pool);
	for i:=0;i<cnt;i++ { 
		go func (){ atomic.AddInt32(&entered,1); _,e:= fetch_proc(pool); if e.Code!=Err_PoolClosed {Fail(t)}; atomic.AddInt32(&fincnt,1);}() 
	}
	for ( entered!=int32(cnt)) { time.Sleep(10*time.Millisecond );}
	pool.Close(true);
	for ( fincnt!=int32(cnt)) { time.Sleep(10*time.Millisecond );}

	if ( st.errors!= st.errors_a[Err_PoolClosed] ) { Fail(t) }

}

func test_Timeout(){
	to := tools.NewTimeoutSig(10*time.Millisecond);
	<-to.C
	
}

func test_Pool(t *testing.T) {
	Pool_long_test();
}
func Test_small_proc(t *testing.T){
	d:=[]byte("r\ntttt\nutuuu\nttt");
	if (3!= tools.Find_count_ofchar(d,'\n')) {Fail(t)}
	if (8!= tools.Find_count_ofchar(d,'t')) {Fail(t)}
	if (0!= tools.Find_count_ofchar(d,'a')) {Fail(t)}
}

func TestMain1(t *testing.T){
	//test_Pool(t)
	test1(t);
	test_Close(t)
	test_Timeout();
}