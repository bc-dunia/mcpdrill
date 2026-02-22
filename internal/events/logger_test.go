package events

import "testing"

func TestGetGlobalEventLoggerReturnsSingletonNoopWhenUnset(t *testing.T) {
	SetGlobalEventLogger(nil)

	a := GetGlobalEventLogger()
	b := GetGlobalEventLogger()

	if a == nil || b == nil {
		t.Fatal("expected non-nil noop logger")
	}
	if a != b {
		t.Fatal("expected singleton noop logger instance")
	}
}
