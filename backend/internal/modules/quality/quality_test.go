package quality

import "testing"

func TestFormat(t *testing.T) {
	if c := Format([]byte(`{"a":1}`), "application/json"); c.Result != ResultPass {
		t.Errorf("valid json: %s", c.Result)
	}
	if c := Format([]byte(`{bad json`), "application/json"); c.Result != ResultFail {
		t.Errorf("invalid json should fail, got %s", c.Result)
	}
	if c := Format([]byte{0xff, 0xfe, 0xfd}, "text/plain"); c.Result != ResultFail {
		t.Errorf("invalid utf-8 should fail, got %s", c.Result)
	}
	if c := Format([]byte("a,b\n1,2\n"), "text/csv"); c.Result != ResultPass {
		t.Errorf("valid csv: %s", c.Result)
	}
}

func TestStats(t *testing.T) {
	c, sample := Stats([]byte("line one\nline two\n\nline four\n"))
	if c.Result != ResultPass {
		t.Errorf("stats result = %s", c.Result)
	}
	if sample != 3 {
		t.Errorf("non-empty lines = %d, want 3", sample)
	}
}

func TestPII(t *testing.T) {
	clean := []byte("这是一段干净的中文训练语料，没有任何个人信息。")
	if c := PII(clean, false); c.Result != ResultPass {
		t.Errorf("clean content should pass, got %s (%v)", c.Result, c.Report)
	}

	withPII := []byte("联系人 张三 手机 13800138000 邮箱 a@b.com 身份证 11010119900101123X")
	if c := PII(withPII, false); c.Result != ResultFail {
		t.Errorf("undeclared PII should fail, got %s (%v)", c.Result, c.Report)
	}
	// Same content but PII was disclosed -> warn, not fail.
	if c := PII(withPII, true); c.Result != ResultWarn {
		t.Errorf("declared PII should warn, got %s", c.Result)
	}
}

func TestSimHash(t *testing.T) {
	a := SimHash([]byte("中文训练数据集合一二三四五六七八九十"))
	b := SimHash([]byte("中文训练数据集合一二三四五六七八九十")) // identical
	c := SimHash([]byte("完全不同的英文 content here totally different"))

	if a != b {
		t.Errorf("identical content must yield identical simhash: %s vs %s", a, b)
	}
	if Hamming(a, b) != 0 {
		t.Errorf("hamming of identical = %d, want 0", Hamming(a, b))
	}
	if Hamming(a, c) <= 0 {
		t.Errorf("different content should have positive hamming distance, got %d", Hamming(a, c))
	}
}
