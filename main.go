package main


import (
	"fmt"
	"net/http"
	"io/ioutil"
	"time"
	"math"
	"sort"
)

const repetitions = 600
const concurrency_level = 25


type duration_slice []time.Duration

func (p duration_slice) Len() int           { return len(p) }
func (p duration_slice) Less(i, j int) bool { return p[i] < p[j]}
func (p duration_slice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }


type benchmark_stats struct {
	failed_requests int
	successful_requests int
	transferred_bytes int
	durations duration_slice
	started_at time.Time
	ended_at time.Time
}


type measurement struct {
	http_reply_size int
	http_status_code int
	duration time.Duration 
}


func NewBenchmarkStats() *benchmark_stats{
	bm := new(benchmark_stats)
	bm.durations = make([]time.Duration, 0, repetitions)
	bm.started_at = time.Now()
	return bm
}

func (bm *benchmark_stats) stop() {
	bm.ended_at = time.Now()
}

func (bm *benchmark_stats) appendMeasurement(m *measurement) {
	if m.http_status_code >= 200 && m.http_status_code <= 299 {
		bm.successful_requests += 1
		bm.durations = append(bm.durations, m.duration)
	} else {
		bm.failed_requests += 1
	}
	bm.transferred_bytes += m.http_reply_size
}

func (bm *benchmark_stats) requestCount() int {
	return bm.failed_requests + bm.successful_requests
}

func (bm *benchmark_stats) printStats() {
	fmt.Println("Successful requests:", bm.successful_requests)
	fmt.Println("Failed requests:", bm.failed_requests)
	fmt.Println("Transferred kilobytes:", bm.transferred_bytes / 1024)
	elapsed_time := bm.elapsedTime()
	fmt.Println("Kilobytes per second:", math.Floor(float64(bm.transferred_bytes) / 1024.0 / elapsed_time.Seconds() ) )
	fmt.Println("Elapsed wall-clock time:", elapsed_time.String())
	fmt.Println("Slowest request:", bm.slowestRequestDuration().String())
	fmt.Println("Median request:", bm.medianRequestDuration().String())
	fmt.Println("Fastest request:", bm.fastestRequestDuration().String())
	fmt.Println("Average request:", bm.averageRequestDuration().String())
	fmt.Println("Standard deviation:", bm.standardDeviation().String())
}


func (bm *benchmark_stats) elapsedTime() time.Duration {
	return bm.ended_at.Sub(bm.started_at)
}

func (bm *benchmark_stats) totalTime() time.Duration {
	var sum time.Duration
	sum = 0
	for _, value := range bm.durations {
		sum += value
	}
	return sum
}

func (bm *benchmark_stats) averageRequestDuration() time.Duration {
	return time.Duration(math.Floor(float64(bm.totalTime().Nanoseconds()) / float64(len(bm.durations))))
}


func (bm *benchmark_stats) slowestRequestDuration() time.Duration {
	max := bm.durations[0]
	for _, value := range bm.durations {
		if max < value {
			max = value
		}
	}
	return max
}


func (bm *benchmark_stats) fastestRequestDuration() time.Duration {
	min := bm.durations[0]
	for _, value := range bm.durations {
		if min > value {
			min = value
		}
	}
	return min
}


func (bm *benchmark_stats) medianRequestDuration() time.Duration {
	length := bm.durations.Len()

	durations := make(duration_slice, length)
	copy(durations, bm.durations)
	sort.Sort(durations)

	return durations[int(length / 2)]
}


func (bm *benchmark_stats) standardDeviation() time.Duration {
	length := float64(bm.durations.Len())
	mean := bm.averageRequestDuration().Nanoseconds()
	sum_squared_delta := 0.0
	delta := 0.0
	for _, value := range bm.durations {
		delta = float64(mean - value.Nanoseconds())
		sum_squared_delta += (delta * delta)
	}

	variance := sum_squared_delta / length
	return time.Duration(math.Floor(math.Sqrt(variance)))
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



func recordMeasurement(url string, result_channel chan *measurement) {
	go func(){
		t1 := time.Now()
		status, size := retrieveUrl(url)
		t2 := time.Now()
		result_channel <- &measurement{size, status, t2.Sub(t1)}
	}()
}


func main() {
	measurement_channel := make(chan *measurement, concurrency_level)
	benchmark := NewBenchmarkStats()
	url := "http://www.google.com/robots.txt"

	for i:= 0; i < concurrency_level; i += 1 {
		recordMeasurement(url, measurement_channel)
	}

	for i:= 0; i < repetitions; i += 1 {
		m := <- measurement_channel
		benchmark.appendMeasurement(m)
		if benchmark.requestCount() < repetitions {
			recordMeasurement(url, measurement_channel)
		}
	}

	benchmark.stop()
	benchmark.printStats()
}
