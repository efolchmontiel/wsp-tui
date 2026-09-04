package giphy

import (
	"context"
	"testing"
	"time"
)

func TestValidateKeyEmpty(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	ok, err := ValidateKey(ctx, "  ")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("empty key should not be valid")
	}
}
