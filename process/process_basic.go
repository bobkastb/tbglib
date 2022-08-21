package process

import (
	"io"
	"os"
	"os/exec"
	"sync"
	buffs "tbglib/buffers"
	"tbglib/pool_ar"
)

// SyncBufferLines используется для чтения из каналов io.Writer
// Например может использоваться для чтения pipe , exec.Cmd.Stdout,exec.Cmd.Stderr
// при поступлении данных генерируется 2 сигнала (в канал и каллбэк )

/* Process_Basic: обертка для встроенной обертки "exec.Cmd"
Основные дополнения:
	1) Потоки stdout,stdin реализованы через buffs.ASyncBufferRdLines.  Это дает возможность ожидать вывода процесса через канал (chan)
	2) Принадлежность к пулу ресурсов.
Основной функционал:
	NewProcess_Basic(name string , arg ...string)*Process_Basic - создание процесса
	IsolateStdError() - разделение буферов stdout,stdin
	IsTerminate() bool  - тест завершения процесса
	Start()  - запуск процесса
	Kill()   - принудительное завершение процесса
	Stop()   - завершение процесса

	UnwantedFromPoll(  ) // Процесс более не нужен пулу
	GetBufferOut( ) // Получить буффер  stdout
	GetBufferErr( )// Получить буффер  stderr
	Write( string )// Запись строки в stdin
	Get_stdout_chan()// Получить сигнальный канал (chan) для stdout
	Get_stderr_chan()// Получить сигнальный канал (chan) для stdin
	ReadAllOut() // Чтение всего содержимого буфера stdout
	Readline() // Чтение одной строки из буфера stdout
	ReadAllErr()// Чтение всего содержимого буфера stderr
	ReadlineErr()// Чтение одной строки из буфера stderr
*/
type Process_Basic struct {
	mu *sync.Mutex  // мьютекс для SyncBufferLines(stdout) , SyncBufferLines(stdin) 
	cmd *exec.Cmd  // go обертка для процессов ОС
	stdin io.WriteCloser // стандартный go pipe (StdInPipe) 
	pool pool_ar.IPoolOfResource // принадлежность к пулу ресурсов, если nil, то процесс вне пула
	status int32
	//termnate_chan chan any //*Process_Basic
}
//func (p* Process_Basic) Lock() { p.mu.Lock() }
//func (p* Process_Basic) Unlock() { p.mu.Unlock() }
func (p* Process_Basic) Getstate() *os.ProcessState  { return p.cmd.ProcessState; }


func (p* Process_Basic) GetTerminateChan() <- chan int {
	return nil;
}
func (p* Process_Basic) TestWorkState() error{
	if p.IsTerminate() { return &pool_ar.EResourceTerminated; }
	if (p.status>1) { return &pool_ar.EResourceTerminating; }
	return nil;
}

// Возвращает true если процесс завершен
func (p* Process_Basic) IsTerminate() bool { 
	return p.cmd.ProcessState !=nil &&  p.cmd.ProcessState.Exited()
}

func (p* Process_Basic) wait() { 
	p.cmd.Wait(); 
	if (p.pool!=nil) { p.pool.Resource_Finalized(p); }
	if ( p.cmd.Stdout !=p.cmd.Stderr ){
		p.GetBufferErr().Close()
	}
	p.GetBufferOut().Close()
}

// Запуск процесса
func (p* Process_Basic) Start() error { 
	e:= p.cmd.Start()
	if (e==nil) { go func () { p.wait();}() }
	return e;
}
// Уничтожение процесса
func (p* Process_Basic) Kill(  )(error) { 
	if (p.status<3) { p.status=3; }
	if (p.pool!=nil) { p.pool.Resource_Finalizing(p); } 
	return p.cmd.Process.Kill()
}

// Команда завершения процесса
func (p* Process_Basic) Stop(  )(error) { 
	if (p.status<2) { p.status=2; }
	if (p.pool!=nil) { p.pool.Resource_Finalizing(p); } 
	return p.cmd.Process.Release()
 }
// Процесс более не нужен пулу
func (p* Process_Basic)  UnwantedFromPoll(  ) {  p.Stop() }
// Получить буффер  stdout
func (p* Process_Basic) GetBufferOut( )(* buffs.ASyncBufferRdLines) { return p.cmd.Stdout.(*buffs.ASyncBufferRdLines); }
// Получить буффер  stderr
func (p* Process_Basic) GetBufferErr( )(* buffs.ASyncBufferRdLines) { return p.cmd.Stderr.(*buffs.ASyncBufferRdLines); }
// Запись строки в stdin
func (p* Process_Basic) Write( s string )(int,error) { return p.stdin.Write( []byte(s) ); }
// Получить сигнальный канал (chan) для stdout
func (p* Process_Basic) Get_stdout_chan() (<-chan int) { return p.GetBufferOut().Sig_chan;  }
// Получить сигнальный канал (chan) для stdin
func (p* Process_Basic) Get_stderr_chan() (<-chan int) { return p.GetBufferErr().Sig_chan;  }
// Чтение всего содержимого буфера stdout
func (p* Process_Basic) ReadAllOut() (string) { return p.GetBufferOut().Read_all();  }
// Чтение одной строки из буфера stdout
func (p* Process_Basic) Readline() (string,error) { return p.GetBufferOut().Readline();  }
// Чтение всего содержимого буфера stderr
func (p* Process_Basic) ReadAllErr() (string) { return p.GetBufferErr().Read_all();  }
// Чтение одной строки из буфера stderr
func (p* Process_Basic) ReadlineErr() (string,error) { return p.GetBufferErr( ).Readline();  }

// Разделить буффер для stdout и stderr
func (p* Process_Basic) IsolateStdError() { 
	if (p.cmd.Stderr == p.cmd.Stdout) { return; }
	p.cmd.Stderr =buffs.NewSyncBufferLines( p.mu , make(chan int,1) )
 }
 
func (p* Process_Basic) InitCmd( name string , arg []string ) {
	p.mu = &sync.Mutex{}
	p.status=1;
	p.cmd = exec.Command( name , arg... )
	p.cmd.Stdout = buffs.NewSyncBufferLines( p.mu , make(chan int,1) )
	p.cmd.Stderr = p.cmd.Stdout;
	//p.stderr_as_out = true;
	//_,e = cmd.StderrPipe();
	p.stdin,_ = p.cmd.StdinPipe(); 

}
func NewProcess_Basic(name string , arg ...string)*Process_Basic{
	p:=&Process_Basic{};
	p.InitCmd( name , arg )
	return p;
}

