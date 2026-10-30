package collect

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/amb1s1/tracefull/internal/trace"
)

// Owner discovery is an explicit additional address-only pass. Close its bounded
// sessions before scheduling normal collection; do not keep hundreds of idle SSH
// connections open across a global ownership/ambiguity decision.
func sweep(ctx context.Context, p *Prepared, dir string, factory Factory) ([]DeviceResult, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make([]DeviceResult, len(p.Plan.SweepDevices))
	jobs := make(chan int)
	failures := make(chan error, len(results))
	var wg sync.WaitGroup
	for i, d := range p.Plan.SweepDevices {
		results[i] = DeviceResult{Name: d.Device.Name, Status: "cancelled", Reason: "not scheduled", Interfaces: []string{}, Commands: []CommandResult{}}
	}
	q := *p
	q.Options.Groups = []string{"ingress"}
	for range p.Options.Workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				if ctx.Err() != nil {
					continue
				}
				// Disjoint indices retain exclusive evidence names across both passes.
				r, err := runDevice(ctx, q, p.Plan.SweepDevices[i], 256+i, dir, factory)
				results[i] = r
				if err != nil {
					failures <- err
					cancel()
				}
			}
		}()
	}
schedule:
	for i := range results {
		select {
		case jobs <- i:
		case <-ctx.Done():
			break schedule
		}
	}
	close(jobs)
	wg.Wait()
	close(failures)
	var err error
	for e := range failures {
		err = errors.Join(err, e)
	}
	if err != nil {
		return results, err
	}
	if ctx.Err() != nil {
		return results, ctx.Err()
	}
	candidates := map[string][]string{}
	for _, d := range results {
		for _, c := range d.Commands {
			if c.Status != "ok" || c.Facts == nil {
				continue
			}
			for _, a := range c.Facts.Addresses {
				candidates[a.Address] = trace.AddUnique(candidates[a.Address], d.Name)
			}
		}
	}
	discovered := map[string][]string{}
	for _, ip := range unmatched(p.Trace) {
		names := candidates[ip]
		slices.Sort(names)
		if len(names) == 1 {
			discovered[names[0]] = append(discovered[names[0]], ip)
		}
	}
	// Update only unmatched identities. Explicit maps and inventory ambiguity keep
	// their higher precedence, and unique ownership is scoped to the sweep coverage.
	update := func(hops []trace.Hop) {
		for i := range hops {
			for ip, m := range hops[i].Matches {
				if m.Method != "unmatched" || len(candidates[ip]) == 0 {
					continue
				}
				names := slices.Clone(candidates[ip])
				slices.Sort(names)
				m = trace.Match{Method: "owner-sweep", Candidates: names}
				if len(names) == 1 {
					m.Device = names[0]
				} else {
					m.Method = "owner-sweep-ambiguous"
				}
				hops[i].Matches[ip] = m
			}
		}
	}
	update(p.Trace.Hops)
	for i := range p.Trace.Paths {
		update(p.Trace.Paths[i].Hops)
	}
	for _, dp := range p.Plan.SweepDevices {
		ips := discovered[dp.Device.Name]
		if len(ips) == 0 {
			continue
		}
		existing := -1
		for i, d := range p.Plan.Devices {
			if d.Device.Name == dp.Device.Name {
				existing = i
				ips = append(ips, d.HopAddresses...)
				break
			}
		}
		next, e := devicePlan(*p, dp.Device, ips, false)
		if e != nil {
			return results, e
		}
		if existing >= 0 {
			p.Plan.Devices[existing] = next
		} else {
			p.Plan.Devices = append(p.Plan.Devices, next)
		}
	}
	if len(p.Plan.Devices) > 256 {
		return results, fmt.Errorf("sweep resolved more than 256 devices")
	}
	p.Plan.Notes = append(p.Plan.Notes, "Owner-sweep matches describe only successful snapshots in the selected inventory scope; missing/failed devices may leave ownership unresolved.")
	return results, nil
}
