package dns

import (
	"math/rand"
	"net"
	"sort"
)

// OrderedSRV returns a count of the results and a map keyed on the order they should be used.
// This based on the records' priority and randomised selection based on their relative weighting.
// The function's inputs are the same as those for net.LookupSRV
// To use in the correct order:
//
// count, orderedSRV, err := OrderedSRV(service, proto, name)
// i := 1
//
//	for  i <= count {
//	  srv := orderedSRV[i]
//	  // Do something such as dial this SRV. If fails move on the the next or break if it succeeds.
//	  i += 1
//	}
func OrderedSRV(service, proto, name string) (int, map[int]*net.SRV, error) {
	_, addrs, err := net.LookupSRV(service, proto, name)
	if err != nil {
		return 0, make(map[int]*net.SRV), err
	}
	index, osrv := orderSRV(addrs)
	return index, osrv, nil
}

func orderSRV(addrs []*net.SRV) (int, map[int]*net.SRV) {
	// Initialise the ordered map
	var o int
	osrv := make(map[int]*net.SRV)

	prioMap := make(map[int][]*net.SRV, 0)
	for _, srv := range addrs {
		prioMap[int(srv.Priority)] = append(prioMap[int(srv.Priority)], srv)
	}

	priorities := make([]int, 0)
	for p := range prioMap {
		priorities = append(priorities, p)
	}

	var count int
	sort.Ints(priorities)
	for _, p := range priorities {
		tos := weightedOrder(prioMap[p])
		for i, s := range tos {
			count += 1
			osrv[o+i] = s
		}
		o += len(tos)
	}
	return count, osrv
}

func weightedOrder(srvs []*net.SRV) map[int]*net.SRV {
	remaining := make([]*net.SRV, len(srvs))
	copy(remaining, srvs)

	var sum int
	for _, s := range remaining {
		sum += int(s.Weight)
	}

	osrv := make(map[int]*net.SRV, len(remaining))

	for o := 1; len(remaining) > 0; o++ {
		i := 0
		if sum > 0 {
			// Select the first record whose running sum of weights exceeds a uniform random number below the sum.
			n := rand.Intn(sum)
			running := 0
			for i = range remaining {
				running += int(remaining[i].Weight)
				if running > n {
					break
				}
			}
		}
		osrv[o] = remaining[i]
		sum -= int(remaining[i].Weight)
		remaining = append(remaining[:i], remaining[i+1:]...)
	}

	return osrv
}
