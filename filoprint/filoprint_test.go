package filoprint

import (
	"context"
	"testing"

	"github.com/crgimenes/filo"
)

func TestBuiltinPrint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []filo.Value
		wantErr bool
	}{
		{
			name:    "single string",
			args:    []filo.Value{filo.VString("hello")},
			wantErr: false,
		},
		{
			name:    "multiple args",
			args:    []filo.Value{filo.VString("value:"), filo.VNum(42)},
			wantErr: false,
		},
		{
			name:    "no args - error",
			args:    []filo.Value{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := builtinPrint(context.Background(), tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("builtinPrint() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBuiltinPrintf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []filo.Value
		wantErr bool
	}{
		{
			name:    "format with args",
			args:    []filo.Value{filo.VString("User %s has %d points"), filo.VString("Alice"), filo.VNum(100)},
			wantErr: false,
		},
		{
			name:    "format with %T",
			args:    []filo.Value{filo.VString("Type is %T"), filo.VNum(42)},
			wantErr: false,
		},
		{
			name:    "format only",
			args:    []filo.Value{filo.VString("Simple message")},
			wantErr: false,
		},
		{
			name:    "no args - error",
			args:    []filo.Value{},
			wantErr: true,
		},
		{
			name:    "non-string format - error",
			args:    []filo.Value{filo.VNum(42)},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := builtinPrintf(context.Background(), tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("builtinPrintf() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFiloTypeName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		val  filo.Value
		want string
	}{
		{"bool", filo.VBool(true), "bool"},
		{"number", filo.VNum(42), "number"},
		{"string", filo.VString("hello"), "string"},
		{"list", filo.VList(nil), "list"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filoTypeName(tt.val)
			if got != tt.want {
				t.Errorf("filoTypeName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRegisterBuiltins(t *testing.T) {
	t.Parallel()

	eng := filo.NewEngine()

	// Should not panic
	RegisterBuiltins(eng)

	// Verify builtins are registered by running a simple script
	globals := map[string]filo.Value{}
	cfg := filo.EvalConfig{StepLimit: 1000, RecursionLimit: 100}

	_, _, err := eng.RunScript(context.Background(), `(print "test")`, globals, cfg)
	if err != nil {
		t.Errorf("RunScript with print failed: %v", err)
	}

	_, _, err = eng.RunScript(context.Background(), `(printf "test %d" 42)`, globals, cfg)
	if err != nil {
		t.Errorf("RunScript with printf failed: %v", err)
	}
}
