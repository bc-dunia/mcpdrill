package mockserver

import "testing"

func TestEvalExpression_DivisionByZeroReturnsError(t *testing.T) {
	_, err := evalExpression("1/0")
	if err == nil {
		t.Fatal("expected division by zero error")
	}
}
