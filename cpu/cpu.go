package main

import (
	"fmt"
	"time"
)

func main() {
	ReportUsage()
}
func ReportUsage() {
	var a uint64 = 18446744073709551615
	fmt.Println(a)
	a=1<<64-1
	fmt.Println(a)
	a=1<<32-1
	fmt.Println(a)
	stats, e := ReadStat("/proc/stat")
	fmt.Println(stats)
	if e != nil {
		fmt.Errorf("Failed to ReadStat: %v", e)
		return
	}
	info := stats.CPUStatAll
	fmt.Println(info)

	cpuChan := NewWatcher(time.Second)
	defer close(cpuChan)

	for {
		select {
		case cpu, ok := <-cpuChan:
			if ok {
				fmt.Println(cpu.Usage)
			}
		}
		fmt.Println("------------")
	}
}

// Event when no error happened, Usage is current cpu usage(eg: 0.5).
type Event struct {
	Usage float64
	Error error
}

// NewWatcher produce one cpu usage num per |dur|; the returned chan
// would be closed when error happened, or close the chan when client code
// want to stop the watcher goroutine.
func NewWatcher(dur time.Duration) chan Event {
	ch := make(chan Event)
	go watchCpu(ch, dur)
	return ch
}

func watchCpu(ch chan Event, dur time.Duration) {
	defer func() {
		if e := recover(); e != nil {
			err := fmt.Errorf("watchCpu failed: %v", e)
			ch <- Event{Usage: 0.0, Error: err}
		}
	}()

	infoPre, err := getCurCpu()
	if err != nil {
		ch <- Event{0.0, err}
		return
	}

	ticker := time.NewTicker(dur)
	defer ticker.Stop()

	for _ = range ticker.C {
		infoCur, err := getCurCpu()
		if err != nil {
			ch <- Event{0.0, err}
			break
		}
		a := infoCur.Idle - infoPre.Idle
		fmt.Println(a)
		usage := float64(a) / float64(a+infoCur.User-infoPre.User+infoCur.Nice+infoCur.System+infoCur.IOWait+infoCur.IRQ+infoCur.SoftIRQ+infoCur.Steal+infoCur.Guest+infoCur.GuestNice-infoPre.Nice-infoPre.System-infoPre.IOWait-infoPre.IRQ-infoPre.SoftIRQ-infoPre.Steal-infoPre.Guest-infoPre.GuestNice)
		ch <- Event{Usage: usage, Error: nil}
		infoPre = infoCur
	}
}

func getCurCpu() (info  CPUStat, err error) {
	stats, e :=  ReadStat("/proc/stat")
	if e != nil {
		err = fmt.Errorf("Failed to ReadStat: %v", e)
		return
	}
	info = stats.CPUStatAll

	return info, nil
}
