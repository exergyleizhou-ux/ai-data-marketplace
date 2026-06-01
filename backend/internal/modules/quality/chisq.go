package quality

import (
	"math"
	"sort"
)

// chiSquareP returns the upper-tail p-value P(X² > stat) for a chi-square
// distribution with df degrees of freedom — i.e. the survival function, which
// equals the regularized upper incomplete gamma Q(df/2, stat/2). Used to turn a
// Benford/terminal-digit chi-square statistic into a calibrated p-value.
func chiSquareP(stat float64, df int) float64 {
	if df <= 0 || stat < 0 || math.IsNaN(stat) {
		return math.NaN()
	}
	if stat == 0 {
		return 1
	}
	return gammaQ(float64(df)/2, stat/2)
}

// gammaQ is the regularized upper incomplete gamma function Q(a,x)=1-P(a,x),
// via series expansion for x<a+1 and a continued fraction otherwise
// (Numerical Recipes §6.2). Stable for the ranges we feed it.
func gammaQ(a, x float64) float64 {
	if x < 0 || a <= 0 {
		return math.NaN()
	}
	if x == 0 {
		return 1
	}
	if x < a+1 {
		return 1 - gammaSeriesP(a, x)
	}
	return gammaCF(a, x)
}

// gammaSeriesP computes the lower regularized incomplete gamma P(a,x) by series.
func gammaSeriesP(a, x float64) float64 {
	gln, _ := math.Lgamma(a)
	ap := a
	sum := 1.0 / a
	del := sum
	for n := 0; n < 1000; n++ {
		ap++
		del *= x / ap
		sum += del
		if math.Abs(del) < math.Abs(sum)*1e-15 {
			break
		}
	}
	return sum * math.Exp(-x+a*math.Log(x)-gln)
}

// gammaCF computes the upper regularized incomplete gamma Q(a,x) by the
// modified Lentz continued fraction.
func gammaCF(a, x float64) float64 {
	gln, _ := math.Lgamma(a)
	const tiny = 1e-300
	b := x + 1 - a
	c := 1 / tiny
	d := 1 / b
	h := d
	for i := 1; i < 1000; i++ {
		an := -float64(i) * (float64(i) - a)
		b += 2
		d = an*d + b
		if math.Abs(d) < tiny {
			d = tiny
		}
		c = b + an/c
		if math.Abs(c) < tiny {
			c = tiny
		}
		d = 1 / d
		del := d * c
		h *= del
		if math.Abs(del-1) < 1e-15 {
			break
		}
	}
	return math.Exp(-x+a*math.Log(x)-gln) * h
}

// benjaminiHochberg returns FDR-adjusted p-values aligned to the input order.
// Controls the false-discovery rate across all simultaneous findings so a
// dataset with many columns is not flagged on chance alone.
func benjaminiHochberg(p []float64) []float64 {
	m := len(p)
	if m == 0 {
		return nil
	}
	type ip struct {
		idx int
		p   float64
	}
	order := make([]ip, m)
	for i, v := range p {
		order[i] = ip{i, v}
	}
	sort.Slice(order, func(i, j int) bool { return order[i].p < order[j].p })

	adj := make([]float64, m)
	prev := 1.0
	for k := m - 1; k >= 0; k-- {
		rank := float64(k + 1)
		val := order[k].p * float64(m) / rank
		if val < prev {
			prev = val
		}
		adj[order[k].idx] = math.Min(prev, 1)
	}
	return adj
}
