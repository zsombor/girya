package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"os"
	"sort"
	"time"
)

type durationSlice []time.Duration

func (p durationSlice) Len() int           { return len(p) }
func (p durationSlice) Less(i, j int) bool { return p[i] < p[j] }
func (p durationSlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

type benchmarkStats struct {
	requestsStarted int
	// concurrent workers running in parallel will feed the
	// results to a single collector trough this buffered channel.
	resultChannel      chan *measurement
	failedRequests     int
	successfulRequests int
	transferredBytes   int
	durations          durationSlice
	startedAt          time.Time
	endedAt            time.Time
}

type measurement struct {
	httpReplySize  int
	httpStatusCode int
	duration       time.Duration
}

func NewBenchmarkStats(repetitions int, concurrencyLevel int) *benchmarkStats {
	bm := new(benchmarkStats)
	bm.durations = make([]time.Duration, 0, repetitions)
	bm.startedAt = time.Now()
	bm.resultChannel = make(chan *measurement, concurrencyLevel)
	return bm
}

func (bm *benchmarkStats) stop() {
	bm.endedAt = time.Now()
}

func (bm *benchmarkStats) recordResult(m *measurement) {
	if m.httpStatusCode >= 200 && m.httpStatusCode <= 299 {
		bm.successfulRequests += 1
		bm.durations = append(bm.durations, m.duration)
	} else {
		bm.failedRequests += 1
	}
	bm.transferredBytes += m.httpReplySize
}

func (bm *benchmarkStats) requestCount() int {
	return bm.failedRequests + bm.successfulRequests
}

func (bm *benchmarkStats) printStats() {
	fmt.Println("Successful requests:", bm.successfulRequests)
	fmt.Println("Failed requests:", bm.failedRequests)
	fmt.Println("Transferred kilobytes:", bm.transferredBytes/1024)
	elapsedTime := bm.elapsedTime()
	fmt.Println("Kilobytes per second:", math.Floor(float64(bm.transferredBytes)/1024.0/elapsedTime.Seconds()))
	fmt.Printf("Elapsed wall-clock time: %.2fs\n", elapsedTime.Seconds())
	fmt.Printf("Slowest request: %.2fs\n", bm.slowestRequestDuration().Seconds())
	fmt.Printf("Median request: %.2fs\n", bm.medianRequestDuration().Seconds())
	fmt.Printf("Fastest request: %.2fs\n", bm.fastestRequestDuration().Seconds())
	fmt.Printf("Average request: %.2fs\n", bm.averageRequestDuration().Seconds())
	fmt.Printf("Standard deviation: %.2fs\n", bm.standardDeviation().Seconds())
}

func (bm *benchmarkStats) elapsedTime() time.Duration {
	return bm.endedAt.Sub(bm.startedAt)
}

func (bm *benchmarkStats) totalTime() time.Duration {
	var sum time.Duration
	sum = 0
	for _, value := range bm.durations {
		sum += value
	}
	return sum
}

func (bm *benchmarkStats) averageRequestDuration() time.Duration {
	return time.Duration(math.Floor(float64(bm.totalTime().Nanoseconds()) / float64(len(bm.durations))))
}

func (bm *benchmarkStats) slowestRequestDuration() time.Duration {
	max := bm.durations[0]
	for _, value := range bm.durations {
		if max < value {
			max = value
		}
	}
	return max
}

func (bm *benchmarkStats) fastestRequestDuration() time.Duration {
	min := bm.durations[0]
	for _, value := range bm.durations {
		if min > value {
			min = value
		}
	}
	return min
}

func (bm *benchmarkStats) medianRequestDuration() time.Duration {
	length := bm.durations.Len()

	durations := make(durationSlice, length)
	copy(durations, bm.durations)
	sort.Sort(durations)

	return durations[int(length/2)]
}

func (bm *benchmarkStats) standardDeviation() time.Duration {
	length := float64(bm.durations.Len())
	mean := bm.averageRequestDuration().Nanoseconds()
	sumDeltaSquared := 0.0
	delta := 0.0
	for _, value := range bm.durations {
		delta = float64(mean - value.Nanoseconds())
		sumDeltaSquared += (delta * delta)
	}

	variance := sumDeltaSquared / length
	return time.Duration(math.Floor(math.Sqrt(variance)))
}

func (bm *benchmarkStats) measureUrl(url string) {
	bm.requestsStarted += 1
	go func() {
		t1 := time.Now()
		status, size := retrieveUrl(url)
		t2 := time.Now()
		bm.resultChannel <- &measurement{size, status, t2.Sub(t1)}
	}()
}

func (bm *benchmarkStats) receiveResult() {
	result := <-bm.resultChannel
	bm.recordResult(result)
}

func retrieveUrl(url string) (int, int) {
	resp, err := http.Get(url)
	if err != nil {
		fmt.Println("failed to fetch ", url)
		return 500, 0
	}

	size := 0
	for header, value := range resp.Header {
		size += len(header) + len(value)
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("failed to read body ...")
		return resp.StatusCode, size
	}

	size += len(body)
	return resp.StatusCode, size
}

func main() {
	// parse command line arguments
	var concurrencyLevel, repetitions int
	flag.IntVar(&concurrencyLevel, "c", 5, "Concurrency level.")
	flag.IntVar(&repetitions, "r", 300, "Number of requests to perform.")
	flag.Parse()

	// url is the first non flag argument. If none exists, print the usage and exit.
	if len(flag.Args()) == 0 {
		fmt.Println("Girya is a simple HTTP stress tester.\n")
		fmt.Println("Usage: gyra [options] URL")
		flag.PrintDefaults()
		fmt.Println("")
		os.Exit(0)
	}
	url := flag.Arg(0)

	// start the benchmark here
	benchmark := NewBenchmarkStats(repetitions, concurrencyLevel)

	// start the given number of requests in parallel
	for i := 0; i < concurrencyLevel; i += 1 {
		benchmark.measureUrl(url)
	}

	for i := 0; i < repetitions; i += 1 {
		// wait till a result arrives, process it and start a
		// new worker if required.
		benchmark.receiveResult()
		if benchmark.requestsStarted < repetitions {
			benchmark.measureUrl(url)
		}
	}

	benchmark.stop()
	benchmark.printStats()
}
