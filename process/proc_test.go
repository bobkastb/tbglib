package process

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	buffs "tbglib/buffers"
	"testing"
	"time"
)

func Fail(t *testing.T) {
	funcName,file,line,ok := runtime.Caller(1);
	//t.Errorf("error at %s func %s line %d %v",file,funcName,line,ok);
	t.Errorf("error at %s func (%d) line %d %v",file,funcName,line,ok);
}


func Test_cmd1(t *testing.T){
	var e error;
	//decoder := charmap.CodePage437.NewDecoder()
	//decoder := charmap.CodePage866.NewDecoder()
	//decoder := charmap.KOI8R.NewDecoder()
	//decoder := charmap.Windows1251.NewDecoder()
	cc := NewProcess_Basic("cmd.exe");
	waiter := cc.Get_stdout_chan()
	e=cc.Start(); if (e!=nil) { fmt.Println(e); return}
	rdans := func(  cntempt int ) (res string) {  for {
			<-waiter
			s,e:=cc.Readline(); 
			if (e!=nil) { fmt.Println("error:",e); return res;}
			//s,e =decoder.String( s ); 
			if (e!=nil) { fmt.Println("error decoder:",e); }
			res += s;
			//fmt.Print(s);
			if (s=="\r\n") {cntempt--; if (cntempt<=0) {return res;}}
	}}
	ans:=rdans(1)
	cc.Write("\n"); <-waiter; promt,_:=cc.Readline(); 
	promt = strings.TrimSpace( promt )
	//ans=rdans(1);
	cc.Write("chcp 1251\n"); 
	ans=rdans(1);
	if (ans != promt+"chcp 1251\nActive code page: 1251\r\n\r\n") { Fail(t); return; }
	//cc.Write("echo %time%\n"); 	rdans(1);
	cc.Write("echo tt\n"); 	ans=rdans(1);
	if (ans != promt+"echo tt\ntt\r\n\r\n") { Fail(t); return; }
	
	go func() { time.Sleep(100*time.Millisecond ); cc.cmd.Process.Kill(); }()
	<-waiter
	//cc.cmd.Wait()
	if !cc.IsTerminate() { Fail(t)}
	fmt.Println("Process terminated")
	
	
}



func Test_TxtStrmWriter(t *testing.T){
	strm := buffs.NewSyncBufferLines( &sync.Mutex{},make(chan int,1) );
	d:= []byte("\nl1\nl2\nl3")
	strm.Write( d );
	fra := func() string { 
		res:=""
		for { 
			if (strm.GetCountLines()>0 && len(strm.Sig_chan)==0) {Fail(t); return res; }
			if strm.GetCountLines()>0 {
			 	<-strm.Sig_chan }
			s,e:=strm.Readline(); 
			if (e!=nil){ 
				return res; }
			res+="-"+s
		}
	}
	if fra()!="-\n-l1\n-l2\n" { Fail(t)}
	strm.Write( d );
	if fra()!="-l3\n-l1\n-l2\n" { Fail(t)}
}


func Test_Proc1(t *testing.T){
	//t.Log("Start test")
	//fmt.Println("fmt:Start test")
}