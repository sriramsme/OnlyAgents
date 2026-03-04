package tools

type ExecControl uint8

const (
	ExecContinue ExecControl = iota
	ExecHalt                 // only used for send_directly_to_user delegation
)

type ToolExecution struct {
	Result        any
	Err           error
	Control       ExecControl
	DirectMessage string // only populated on ExecHalt
}

func ExecOK(result any) ToolExecution {
	return ToolExecution{Result: result, Control: ExecContinue}
}

func ExecErr(err error) ToolExecution {
	return ToolExecution{Err: err, Control: ExecContinue}
}

func ExecDone(msg string) ToolExecution {
	return ToolExecution{DirectMessage: msg, Control: ExecHalt}
}
