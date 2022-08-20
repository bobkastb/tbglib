package tools

type ErrorWithCode struct {
	Txt   string
	Code  int
	Level int
}

func (e *ErrorWithCode) Error() string { return e.Txt }

//type ILogErrorEvent interface { LogErrorEvent( e *ErrorBasicPool ); }
type ILogErrorEvent interface{ LogErrorEvent(e error) }
