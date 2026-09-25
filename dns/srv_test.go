package dns

import (
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOrderSRV(t *testing.T) {
	srv11 := net.SRV{
		Target:   "t11",
		Port:     1234,
		Priority: 1,
		Weight:   100,
	}
	srv12 := net.SRV{
		Target:   "t12",
		Port:     1234,
		Priority: 1,
		Weight:   100,
	}
	srv13 := net.SRV{
		Target:   "t13",
		Port:     1234,
		Priority: 1,
		Weight:   20,
	}
	srv21 := net.SRV{
		Target:   "t21",
		Port:     1234,
		Priority: 2,
		Weight:   1,
	}

	addrs := []*net.SRV{
		&srv11, &srv21, &srv12, &srv13,
	}
	count, orderedSRV := orderSRV(addrs)
	assert.Equal(t, len(addrs), count, "Index not the expected size")
	assert.Equal(t, len(addrs), len(orderedSRV), "orderedSRV not the expected size")
	assert.Equal(t, uint16(2), orderedSRV[4].Priority, "Priority order not as expected")
}

func TestWeightedOrderIsProportionalToWeight(t *testing.T) {
	testCases := []struct {
		name     string
		weights  []uint16
		first    int
		expected float64
	}{
		{"ShouldPickLightFirstAQuarterOfTheTime", []uint16{1, 3}, 0, 0.25},
		{"ShouldPickHeavyFirstMostOfTheTime", []uint16{1, 1, 98}, 2, 0.98},
		{"ShouldPickZeroWeightLast", []uint16{0, 10}, 1, 1},
	}

	const trials = 100000

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var hits int

			for range trials {
				srvs := make([]*net.SRV, len(tc.weights))
				for i, w := range tc.weights {
					srvs[i] = &net.SRV{Target: fmt.Sprintf("host%d.", i), Weight: w}
				}

				first := srvs[tc.first]

				if weightedOrder(srvs)[1] == first {
					hits++
				}
			}

			assert.InDelta(t, tc.expected, float64(hits)/trials, 0.01)
		})
	}
}

func TestWeightedOrderDoesNotReorderInput(t *testing.T) {
	srvs := []*net.SRV{{Target: "a.", Weight: 1}, {Target: "b.", Weight: 2}, {Target: "c.", Weight: 3}}
	want := append([]*net.SRV{}, srvs...)

	for range 100 {
		weightedOrder(srvs)
	}

	assert.Equal(t, want, srvs)
}
