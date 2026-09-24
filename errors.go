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

// PositionError is an error of a script compiled from source, and where it
// happened: the line and the column (both from 1, the column in bytes) of the
// innermost expression that failed to run, or that did not compile. The
// message is the one the error always had, and Unwrap reaches it.
type PositionError struct {
	Line int
	Col  int
	Err  error
}

func (e *PositionError) Error() string {
	return e.Err.Error()
}

func (e *PositionError) Unwrap() error {
	return e.Err
}
