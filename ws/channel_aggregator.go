package main
import (
	"time"
)

// Channel-based aggregator that reports the global average temperature periodically
//
// Report the averagage temperature across all `k` weatherstations every `averagePeriod`
// seconds by sending a `WeatherReport` struct to the `out` channel. The aggregator should
// terminate upon receiving a singnal on the `quit` channel.
//
// Note! To receive credit, channelAggregator must not use mutexes.
func channelAggregator(
	k int,
	averagePeriod float64,
	getWeatherData func(int, int) WeatherReport,
	out chan WeatherReport,
	quit chan struct{},
) {
	// Your code here.

	ticker := time.NewTicker(time.Duration(averagePeriod * float64(time.Second)))
	reports := make(chan WeatherReport)

	defer ticker.Stop()

	batch := 0
	for i := 0; i < k; i++ {
		go func(stationID int, b int) {
			reports <- getWeatherData(stationID, b)
		}(i, batch)
	}

	var total float64
	var count int
	for {
		select{
			case <-ticker.C:
				average := total / float64(count)
				out <- WeatherReport{Value: average, Batch: batch}
				total = 0
				count = 0
				batch++
				for i := 0; i < k; i++ {
					go func(stationID int, b int) {
						reports <- getWeatherData(stationID, b)
					}(i, batch)
				}
			case report := <-reports:
					if report.Batch == batch {
						total += report.Value
						count++
					}
			case <-quit:
				return
		}
	}

}
