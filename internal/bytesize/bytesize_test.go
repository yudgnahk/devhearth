package bytesize

import "testing"

func TestFormat(t *testing.T) {
	cases := []struct {
		name  string
		value int64
		want  string
	}{
		{name: "zero", value: 0, want: "0 B"},
		{name: "bytes", value: 512, want: "512 B"},
		{name: "kibibytes", value: 4096, want: "4.0 KiB"},
		{name: "mebibytes", value: 5 * 1024 * 1024, want: "5.0 MiB"},
		{name: "gibibytes", value: 3 * 1024 * 1024 * 1024, want: "3.0 GiB"},
		{name: "tebibytes", value: 2 * 1024 * 1024 * 1024 * 1024, want: "2.0 TiB"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := Format(testCase.value); got != testCase.want {
				t.Fatalf("Format(%d) = %q, want %q", testCase.value, got, testCase.want)
			}
		})
	}
}

func TestFormatRange(t *testing.T) {
	if got := FormatRange(1024, 1024); got != "1.0 KiB" {
		t.Fatalf("equal bounds should collapse, got %q", got)
	}
	if got := FormatRange(1024, 4096); got != "1.0 KiB–4.0 KiB" {
		t.Fatalf("range = %q", got)
	}
}
