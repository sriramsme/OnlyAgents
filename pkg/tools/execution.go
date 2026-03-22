package tools

import (
	"reflect"
)

type ExecControl uint8

const (
	ExecContinue ExecControl = iota
	ExecHalt                 // only used for send_directly_to_user delegation
)

type ToolExecution struct {
	Result        any
	Err           error
	Control       ExecControl
	DirectMessage string   // only populated on ExecHalt
	ProducedFiles []string // local paths of files written to disk
}

func ExecOK(result any) ToolExecution {
	return ToolExecution{Result: normalizeResult(result), Control: ExecContinue}
}

func ExecErr(err error) ToolExecution {
	return ToolExecution{Err: err, Control: ExecContinue}
}

func ExecDone(msg string) ToolExecution {
	return ToolExecution{DirectMessage: msg, Control: ExecHalt}
}

func (t ToolExecution) IsHalt() bool { return t.Control == ExecHalt }
func (t ToolExecution) IsErr() bool  { return t.Err != nil }

// normalizeResult converts nil slices to empty slices so LLMs receive
// "[]" instead of "null" in tool results.
func normalizeResult(v any) any {
	if v == nil {
		return v
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice && rv.IsNil() {
		return reflect.MakeSlice(rv.Type(), 0, 0).Interface()
	}
	return v
}
