package tools

const (
	ErrorLevel   = -200
	WarningLevel = -100
	NormalLevel  = 0
	VerboseLevel = 100
	DebugLevel   = 200
)

type ILevelLogger interface {
	LPrintf(level int, format string, v ...interface{})
}

func GetDefaultLogger() ILevelLogger { return nil }
