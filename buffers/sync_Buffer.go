package buffers

import (
	"bytes"
	"io"
	"sync"
	"sync/atomic"
	"tbglib/tools"
)

/*
func NewSyncBufferLines( mu *sync.Mutex , sig_chan chan int ) *ASyncBufferLines{
func (s*ASyncBufferLines) Readline() (res string , err error)
func (s*ASyncBufferLines) Read_all() string
func (s*ASyncBufferLines) Write(buff []byte) (n int, err error)
func (s* ASyncBufferLines) Lock()
func (s* ASyncBufferLines) Unlock()
func (s* ASyncBufferLines) Close()
func (s* ASyncBufferLines) GetCountLines()
*/

// ASyncBufferRdLines используется для асинхронного чтения из объектов с интерфейсом io.Writer
// Например может использоваться для чтения pipe , exec.Cmd.Stdout, exec.Cmd.Stderr
// При поступлении данных (вызове Write(buff []byte)) генерируется 2 сигнала (в канал Sig_chan и каллбэк Sig_cb )
// Это позволяет асинхронно ожидать появления данных в буфере (чтением из канала Sig_chan)
// Достаточно подставить ASyncBufferRdLines стандарной обертке процесса в go , и можно ожидать данных из его Stdout с помошью ожидания канала
type ASyncBufferRdLines struct {
	mu *sync.Mutex
	//buff []string;
	Sig_chan chan int // сигнальный канал пользователя буфера. В канале данные если бучер непуст
	Sig_cb  func (); // сигнальный колбэк пользователя буфера
	buffer *bytes.Buffer
	cnt_lines int32
	sep_char byte
	_isclosed bool
	//reader *bufio.Reader
	//writer *bufio.Writer
}


func NewSyncBufferLines( mu *sync.Mutex , sig_chan chan int ) *ASyncBufferRdLines{
	res:= & ASyncBufferRdLines{
		mu:mu, Sig_chan:sig_chan , 
		buffer: bytes.NewBufferString(""),
		cnt_lines:0,
		sep_char:'\n',
		_isclosed:false,
	}
	//rw := bufio.NewReadWriter()
	return res
}

// Возвращает количество строк в буфере
func (s* ASyncBufferRdLines) GetCountLines() int { return int(s.cnt_lines); }


func (s* ASyncBufferRdLines) Lock() { if (s.mu!=nil) { s.mu.Lock() } }
func (s* ASyncBufferRdLines) Unlock() { if (s.mu!=nil) {s.mu.Unlock()} }

// Закрывает буффер. Перестает принимать данные и закрывает сигнальный канал
func (s* ASyncBufferRdLines) Close() { 
	s.Lock(); defer s.Unlock();
	s._isclosed = true;
	close(s.Sig_chan);
 }

 // Возвращает состояние буфера. True - буффер закрыт
func (s* ASyncBufferRdLines) IsClose() bool { 
	return s._isclosed ;
}
func (s*ASyncBufferRdLines) push_signal( tocb bool ){
	
	if (s.Sig_chan!=nil && !s.IsClose()) {
		select { 
		case s.Sig_chan<-int(s.cnt_lines):
		default:		
		}	
	}
	if (tocb && s.Sig_cb!=nil) { s.Sig_cb() }
}
func (s*ASyncBufferRdLines) reset_chan(  ){
	s.cnt_lines = 0;
	if (s.Sig_chan==nil || s.IsClose()) {return}
	for {
		select { 
		case  <-s.Sig_chan:
		default: return;		
		}	
	}
}


// Запись данных в буффер. Генерирует сигналы.	
func (s*ASyncBufferRdLines) Write(buff []byte) (n int, err error) {
	s.Lock(); defer s.Unlock();
	//TODO: decoder???
	cntl := tools.Find_count_ofchar(buff,s.sep_char);
	//s.cnt_lines += cntl;
	atomic.AddInt32( &s.cnt_lines , int32(cntl) );
	n,err = s.buffer.Write( buff )
	if (cntl>0) { s.push_signal( true ); }
	return; 
}

// Чтение всего буфера в строку. Если буфер пуст возвращает пустую строку и ошибку io.EOF
func (s*ASyncBufferRdLines) Read_all() string {
	s.Lock(); defer s.Unlock();
	res:=s.buffer.String()
	s.buffer.Truncate(0);
	s.cnt_lines = 0;
	s.reset_chan()
	return res
}

// Чтение одной строки буффера. Если буфер пуст возвращает пустую строку и ошибку io.EOF
func (s*ASyncBufferRdLines) Readline() (res string , err error) {
	s.Lock(); defer s.Unlock();
	//res,err = s.buffer.ReadString(s.sep_char) ;
	if (s.cnt_lines>0) {	
			res,err = s.buffer.ReadString(s.sep_char);
			//s.cnt_lines--;
			atomic.AddInt32( &s.cnt_lines , -1 );

		} else {  
		s.cnt_lines=0;err= io.EOF
		} 
	if (s.cnt_lines>0) { s.push_signal( false ); 
		} else { s.reset_chan() }
	return res,err
}
