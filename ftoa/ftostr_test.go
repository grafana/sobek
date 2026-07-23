package ftoa

import (
	"math"
	"math/rand"
	"strconv"
	"testing"
)

func _testFToStr(num float64, mode FToStrMode, precision int, expected string, t *testing.T) {
	buf := FToStr(num, mode, precision, nil)
	if s := string(buf); s != expected {
		t.Fatalf("expected: '%s', actual: '%s", expected, s)
	}
	if !math.IsNaN(num) && num != 0 && !math.Signbit(num) {
		_testFToStr(-num, mode, precision, "-"+expected, t)
	}
}

func testFToStr(num float64, mode FToStrMode, precision int, expected string, t *testing.T) {
	t.Run("", func(t *testing.T) {
		t.Parallel()
		_testFToStr(num, mode, precision, expected, t)
	})
}

func TestDtostr(t *testing.T) {
	testFToStr(0, ModeStandard, 0, "0", t)
	testFToStr(1, ModeStandard, 0, "1", t)
	testFToStr(9007199254740991, ModeStandard, 0, "9007199254740991", t)
	testFToStr(math.MaxInt64, ModeStandardExponential, 0, "9.223372036854776e+18", t)
	testFToStr(1e-5, ModeFixed, 1, "0.0", t)
	testFToStr(8.85, ModeExponential, 2, "8.8e+0", t)
	testFToStr(885, ModeExponential, 2, "8.9e+2", t)
	testFToStr(25, ModeExponential, 1, "3e+1", t)
	testFToStr(1e-6, ModeFixed, 7, "0.0000010", t)
	testFToStr(math.Pi, ModeStandardExponential, 0, "3.141592653589793e+0", t)
	testFToStr(math.Inf(1), ModeStandard, 0, "Infinity", t)
	testFToStr(math.NaN(), ModeStandard, 0, "NaN", t)
	testFToStr(math.SmallestNonzeroFloat64, ModeExponential, 40, "4.940656458412465441765687928682213723651e-324", t)
	testFToStr(3.5844466002796428e+298, ModeStandard, 0, "3.5844466002796428e+298", t)
	testFToStr(math.Float64frombits(0x0010000000000000), ModeStandard, 0, "2.2250738585072014e-308", t) // smallest normal
	testFToStr(math.Float64frombits(0x000FFFFFFFFFFFFF), ModeStandard, 0, "2.225073858507201e-308", t)  // largest denormal
	testFToStr(4294967272.0, ModePrecision, 14, "4294967272.0000", t)
}

func BenchmarkDtostrSmall(b *testing.B) {
	var buf [128]byte
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		FToStr(math.Pi, ModeStandardExponential, 0, buf[:0])
	}
}

func BenchmarkDtostrShort(b *testing.B) {
	var buf [128]byte
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		FToStr(3.1415, ModeStandard, 0, buf[:0])
	}
}

func BenchmarkDtostrFixed(b *testing.B) {
	var buf [128]byte
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		FToStr(math.Pi, ModeFixed, 4, buf[:0])
	}
}

func BenchmarkDtostrBig(b *testing.B) {
	var buf [128]byte
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		FToStr(math.SmallestNonzeroFloat64, ModeExponential, 40, buf[:0])
	}
}

func BenchmarkAppendFloatBig(b *testing.B) {
	var buf [128]byte
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		strconv.AppendFloat(buf[:0], math.SmallestNonzeroFloat64, 'e', 40, 64)
	}
}

func BenchmarkAppendFloatSmall(b *testing.B) {
	var buf [128]byte
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		strconv.AppendFloat(buf[:0], math.Pi, 'e', -1, 64)
	}
}

// TestDtostrShortestRoundTrip pins the mode-0 (shortest) contract required by
// ECMAScript Number::toString: the produced string must parse back to exactly
// the input float64. Before the fix, the closest-digit choice in the bignum
// fallback propagated a carry whenever rounding up landed ON '9' (a mis-port
// of dtoa.c's `dig++ == '9'` post-increment test), producing a one-digit-short
// string outside the half-ulp interval for ~1 in 15,000 doubles.
func TestDtostrShortestRoundTrip(t *testing.T) {
	// Known pre-fix corruptions (all end in a round-up-to-9 final digit).
	for _, d := range []float64{
		0.7016570306969449, // rendered "0.701657030696945" before the fix
		0.24414061428229689,
		0.5084920399817559,
		0.19243255639630719,
		0.5342431914336429,
		0.9868917361939819,
	} {
		got := string(FToStr(d, ModeStandard, 0, nil))
		want := strconv.FormatFloat(d, 'g', -1, 64)
		if back, _ := strconv.ParseFloat(got, 64); back != d {
			t.Errorf("FToStr(%v, ModeStandard) = %q, does not round-trip (Go shortest: %q)", d, got, want)
		}
	}

	// Differential check against the Go standard library's shortest formatter
	// (a round-trip oracle) over a deterministic pseudo-random sample.
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 500000; i++ {
		d := rng.Float64()
		got := string(FToStr(d, ModeStandard, 0, nil))
		if back, err := strconv.ParseFloat(got, 64); err != nil || back != d {
			t.Fatalf("FToStr(%v, ModeStandard) = %q does not round-trip (err=%v)", d, got, err)
		}
	}
}
