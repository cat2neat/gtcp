package gtcp_test

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/cat2neat/gtcp"
)

func TestTrafficStatistics(t *testing.T) {
	t.Parallel()
	dc := &debugNetConn{}
	dc.ReadFunc = func(buf []byte) (int, error) {
		copy(buf, smashingStr)
		return len(smashingStr), nil
	}
	dc.WriteFunc = func(buf []byte) (int, error) {
		return len(buf), nil
	}
	c := gtcp.NewStatsConn(gtcp.NewBaseConn(dc))
	buf := make([]byte, 4)
	c.Read(buf)
	c.Write([]byte(smashingStr))

	const max = 64
	ts := gtcp.TrafficStatistics{}
	wg := sync.WaitGroup{}
	for i := 0; i < max; i++ {
		wg.Add(1)
		go func() {
			ts.AddConnStats(c)
			wg.Done()
		}()
	}
	wg.Wait()
	decoded := struct {
		InBytes  int `json:"in_bytes"`
		OutBytes int `json:"out_bytes"`
	}{}
	err := json.Unmarshal([]byte(ts.String()), &decoded)
	if err != nil {
		t.Errorf("gtcp_test: TrafficStatistics.String err: %+v\n", err)
	}
	if decoded.InBytes != 4*64 || decoded.OutBytes != 4*64 {
		t.Errorf("gtcp_test: TrafficStatistics.String raw: %s expected: 256, 256 actual: %d, %d\n",
			ts.String(),
			decoded.InBytes,
			decoded.OutBytes)
	}
	ts.Reset()
	err = json.Unmarshal([]byte(ts.String()), &decoded)
	if err != nil {
		t.Errorf("gtcp_test: TrafficStatistics.String err: %+v\n", err)
	}
	if decoded.InBytes != 0 || decoded.OutBytes != 0 {
		t.Errorf("gtcp_test: TrafficStatistics.String raw: %s expected: 0, 0 actual: %d, %d\n",
			ts.String(),
			decoded.InBytes,
			decoded.OutBytes)
	}
}
