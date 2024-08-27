package main
import (
	"time"
	"sync"
)

// Mutex-based aggregator that reports the global average temperature periodically
//
// Report the averagage temperature across all `k` weatherstations every `averagePeriod`
// seconds by sending a `WeatherReport` struct to the `out` channel. The aggregator should
// terminate upon receiving a singnal on the `quit` channel.
//
// Note! To receive credit, mutexAggregator must implement a mutex based solution.
func mutexAggregator(
	k int,
	averagePeriod float64,
	getWeatherData func(int, int) WeatherReport,
	out chan WeatherReport,
	quit chan struct{},
) {
	// Your code here.
    ticker := time.NewTicker(time.Duration(averagePeriod * float64(time.Second)))
    defer ticker.Stop()

    var m sync.Mutex
    var total float64
    var count int

    batch := 0
    for i := 0; i < k; i++ {
        go func(stationID int, b int) {
            report := getWeatherData(stationID, b)
            m.Lock()
            if report.Batch == batch {
                total += report.Value
                count++
            }
            m.Unlock()
        }(i, batch)
    }

    for {
        select {
        case <-ticker.C:
            m.Lock()
            average := total / float64(count)
            out <- WeatherReport{Value: average, Batch: batch}
            total = 0.0
            count = 0
            batch++
            for i := 0; i < k; i++ {
				go func(stationID int, b int) {
					report := getWeatherData(stationID, b)
					m.Lock()
					if report.Batch == batch {
						total += report.Value
						count++
					}
					m.Unlock()
					}(i, batch)
				}
			m.Unlock()
        case <-quit:
            return
        }
    }

}