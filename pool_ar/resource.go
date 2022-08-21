package pool_ar

import (
	"tbglib/tools"
	"time"
)


var EResourceTerminated= tools.ErrorWithCode{ Txt:"Resource terminated" };
var EResourceTerminating= tools.ErrorWithCode{ Txt:"Resource terminating" };

const ( Erest_Nothing =0 ; Erest_Work=1 ; Erest_Finalizing=2 ;Erest_Finalized=3 )
type Trest_status int32;

// Интерфейс ресурса, которым управляет пул. 
type ISingleResource interface{
	// тестирует состояние ресурса. Если ресурс в нерабочем состоянии возвращается ошибка EResourceTerminated|EResourceTerminating
	// При любой ошибке ресурс будет закрыт
	TestWorkState() error 
	Kill() error  // Немедленное уничтожение ресурса
	Stop() error  // Команда закрытия ресурса
	IsTerminate() bool // Возвращает true если ресурс закрыт
	GetTerminateChan() <- chan int; // Возвращает канал для ожидания закрытия ресурса

	// UnwantedFromPoll: Соощение ресурсу о том что его не хотят более использовать. Это не страшно. Просто был лишний "возврат" ресурса (Release): Err_ReleasePullFull и 
	//Нужно закрыть ресурс. Ресурс попытался вернуться а pool уже полный. Ресурс остается Erespool_Free , то есть его нельзя вернуть(Release), и необходимо закрывать.
	UnwantedFromPoll(); 
}

type ITask interface{}
type ITaskResult interface{}

type RTaskDefinitionsData struct {
	Task_default_timeout time.Duration // Максимальное время выполнения задачи
	Resourse_create_tumeout time.Duration // Максимальное для создания процесса/ресурса
	Resourse_close_tumeout time.Duration // Максимальное для завершения процесса/ресурса
	Allow_kill_resource bool // Разрешает не ждать завершения ресурса и использовать Kill
	Tasks_in chan ITask;  // Очередь задач
	Tasks_out chan ITaskResult; // Очередь результатов
}

type ITaskDefinition interface{
	//NewResource: Создает новый ресурс pool может быть nil
	NewResource( pool IPoolOfResource ) (ISingleResource,error)
	//ProcHandler: Обработчик задачи. Не забудьте спросить у ProcWorker про прерыватели (GetInterrupts) и вызвать onInterruptSignal если прерыватели сработали
	ProcHandler( own *ProcWorker , task ITask  ) (ITaskResult,error)
	// Ошибка должна быть обработана , результат обработки нужно вернуть. Позже ProcWorker, сам положит результат в tasks_out
	HandleDOS( task ITask, e error) ITaskResult
	// Если задача устала ждать в очереди,OnPopTaskFromQue должен вернуть false, в этом случае обработка задачи не произойдет
	OnPopTaskFromQue( task ITask) bool
	// Получить все данные (RTaskDefinitionsData)
	GetTaskDefinitionsData() RTaskDefinitionsData;
}


