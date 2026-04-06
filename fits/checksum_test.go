package fits

import (
	"testing"
)

func TestCalcChecksum(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07}
	// length = 7. bytes:
	// 0x01020304
	// + 0x05060700 (padded last 3 bytes)
	// 16909060 + 84281088 = 101190148
	sum := CalcChecksum(data)
	if sum != 101190148 {
		t.Errorf("expected 101190148, got %d", sum)
	}
}

func TestValidateDatasum(t *testing.T) {
	err := ValidateDatasum("123456", 123456)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}

	err = ValidateDatasum("invalid", 123)
	if err == nil {
		t.Errorf("expected error on invalid datasum")
	}

	err = ValidateDatasum("123", 456)
	if err == nil {
		t.Errorf("expected mismatch error")
	}
}

func TestVerifyChecksum(t *testing.T) {
	if !VerifyChecksum(0xFFFFFFFF) {
		t.Errorf("expected true for 0xFFFFFFFF")
	}
	if !VerifyChecksum(0x00000000) {
		t.Errorf("expected true for 0")
	}
	if VerifyChecksum(0x12345678) {
		t.Errorf("expected false for random val")
	}
}
