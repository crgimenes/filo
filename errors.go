package filo

// exitSignal is a sentinel error used to signal early script termination.
// It carries the value to be returned from the script.
type exitSignal struct {
	Value Value
}

func (e *exitSignal) Error() string {
	return "exit"
}

// returnSignal is a sentinel error used to signal early function return.
// It carries the value to be returned from the function.
type returnSignal struct {
	Value Value
}

func (r *returnSignal) Error() string {
	return "return"
}
