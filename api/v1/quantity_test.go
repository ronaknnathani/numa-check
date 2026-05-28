package v1

import "testing"

func TestFormatCPUQuantity(t *testing.T) {
	tests := []struct {
		name  string
		cores float64
		want  string
	}{
		{"whole 4 cores", 4.0, "4"},
		{"whole 1 core", 1.0, "1"},
		{"half core -> 500m", 0.5, "500m"},
		{"1.5 cores -> 1500m", 1.5, "1500m"},
		{"100 millicores", 0.1, "100m"},
		{"single millicore", 0.001, "1m"},
		{"rounds to nearest milli", 0.1004, "100m"},
		{"zero", 0, "0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatCPUQuantity(tt.cores); got != tt.want {
				t.Errorf("FormatCPUQuantity(%v) = %q, want %q", tt.cores, got, tt.want)
			}
		})
	}
}

func TestFormatMemoryQuantity(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{"16 GiB", 16 << 30, "16Gi"},
		{"1 KiB", 1024, "1Ki"},
		{"512 MiB", 512 << 20, "512Mi"},
		{"2 TiB", 2 << 40, "2Ti"},
		{"non-clean bytes falls back to raw", 1025, "1025"},
		{"zero", 0, "0"},
		{"negative falls back to raw", -1, "-1"},
		{"prefers largest clean suffix (3 GiB)", 3 << 30, "3Gi"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatMemoryQuantity(tt.bytes); got != tt.want {
				t.Errorf("FormatMemoryQuantity(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

func TestFormatIntQuantity(t *testing.T) {
	if got := FormatIntQuantity(0); got != "0" {
		t.Errorf("FormatIntQuantity(0) = %q, want %q", got, "0")
	}
	if got := FormatIntQuantity(1); got != "1" {
		t.Errorf("FormatIntQuantity(1) = %q, want %q", got, "1")
	}
	if got := FormatIntQuantity(8); got != "8" {
		t.Errorf("FormatIntQuantity(8) = %q, want %q", got, "8")
	}
}
