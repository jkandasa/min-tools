package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/fvbommel/sortorder"
	"github.com/gdamore/tcell/v2"
	"github.com/minio/madmin-go/v3"
	"github.com/rivo/tview"
)

// Wrap "Info" message together with fields "Status" and "Error"
type clusterStruct struct {
	Status string             `json:"status"`
	Error  string             `json:"error,omitempty"`
	Info   madmin.InfoMessage `json:"info,omitempty"`
}

type driveStatus struct {
	SetIndex   int
	DriveIndex int
	Path       string
	Status     string
	UsedSpace  uint64
	TotalSpace uint64
	UsedInodes uint64
	FreeInodes uint64
	Metrics    *madmin.DiskMetrics
}

func main() {
	if len(os.Args) == 1 {
		fmt.Println("Please provide the filename")
		return
	}

	domainString := ""
	if len(os.Args) >= 3 {
		domainString = strings.TrimSpace(os.Args[2])
	}

	filename := os.Args[1]
	data, err := os.ReadFile(filename)
	if err != nil {
		fmt.Printf("Error on reading the file:%s, err:%v\n", filename, err)
		return
	}

	_driveStatus := map[int]map[string]int{}

	// check raw prefix before unmarshaling
	data = []byte(strings.Replace(string(data), `{"version":"3"}`, "", 1))

	infoStruct := clusterStruct{}
	err = json.Unmarshal(data, &infoStruct)
	if err != nil {
		fmt.Printf("Error on unmarshal, filename:%s\n, err:%v\n", filename, err)
		return
	}

	// if there is no server found on the first try, trying with different format
	// data could be from subnet diagnostics page
	if len(infoStruct.Info.Servers) == 0 {
		anotherFormat := struct {
			InfoStruct clusterStruct `json:"minio"`
		}{}
		err = json.Unmarshal(data, &anotherFormat)
		if err != nil {
			fmt.Printf("Error on unmarshal, filename:%s\n, err:%v\n", filename, err)
		}
		infoStruct = anotherFormat.InfoStruct
	}

	// ec set index => endpoint => disk status
	pools := map[int]map[int]map[string]driveStatus{}
	for _, server := range infoStruct.Info.Servers {
		endpointName := trimDomainData(server.Endpoint, domainString)
		for _, disk := range server.Disks {
			ds := driveStatus{
				SetIndex:   disk.SetIndex,
				Path:       disk.DrivePath,
				DriveIndex: disk.DiskIndex,
				UsedSpace:  disk.UsedSpace,
				TotalSpace: disk.TotalSpace,
				UsedInodes: disk.UsedInodes,
				FreeInodes: disk.FreeInodes,
				Status:     disk.State,
				Metrics:    disk.Metrics,
			}

			// update endpoint name with drive path
			endpointNameWithDrive := fmt.Sprintf("%s:%s", endpointName, disk.DrivePath)
			if disk.DrivePath == "" {
				u, err := url.Parse(disk.Endpoint)
				if err != nil {
					fmt.Printf("Error parsing disk endpoint[%s]: %v\n", disk.Endpoint, err)
				} else {
					endpointNameWithDrive = fmt.Sprintf("%s:%s", endpointName, u.Path)
				}
			}
			poolIndex := disk.PoolIndex
			setIndex := disk.SetIndex

			ecStatus, ok := pools[poolIndex]

			if !ok {
				// pools = append(pools, make(map[int]map[string]driveStatus))
				ecStatus = make(map[int]map[string]driveStatus)
			}

			// fmt.Println("pool index:", poolIndex)
			// ecStatus := pools[poolIndex]

			diskStatus, ok := ecStatus[setIndex]
			if !ok {
				diskStatus = map[string]driveStatus{}
			}
			diskStatus[endpointNameWithDrive] = ds
			ecStatus[setIndex] = diskStatus

			pools[poolIndex] = ecStatus
		}
	}

	for poolIndex, ecStatus := range pools {
		// print server information
		fmt.Printf("\nPool=%d, Servers\n", poolIndex+1)
		serverNames := []string{}
		serversData := map[string]madmin.ServerProperties{}
		for _, server := range infoStruct.Info.Servers {
			endpointName := trimDomainData(server.Endpoint, domainString)
			serverNames = append(serverNames, endpointName)
			serversData[endpointName] = server
		}

		// sort server names
		slices.Sort(serverNames)

		for _, serverName := range serverNames {
			server, found := serversData[serverName]
			if !found {
				fmt.Printf("%s: (data_not_available)\n", serverName)
				continue
			}
			if server.PoolNumber == poolIndex+1 {
				fmt.Printf("%s: (%s)\n", serverName, server.State)
				if server.State == "offline" {
					fmt.Println()
					continue
				}
				fmt.Printf("edition=%s, version=%s, commit_id=%s\n", server.Edition, server.Version, server.CommitID)
				fmt.Printf("mem_stats_[alloc=%s, total=%s], ilm_expiry_in_progress=%v, uptime=%s\n", humanize.IBytes(server.MemStats.Alloc), humanize.IBytes(server.MemStats.TotalAlloc), server.ILMExpiryInProgress, humanizeDuration(time.Duration(server.Uptime)*time.Second))
				fmt.Println()
			}
		}

		// print state
		setIndices := []int{}
		for setIndex := range ecStatus {
			setIndices = append(setIndices, setIndex)
		}
		sort.Ints(setIndices)

		for _, setIndex := range setIndices {
			diskStatus := ecStatus[setIndex]
			fmt.Printf("\nPool=%d, ES=%d\n", poolIndex+1, setIndex+1)
			endpoints := []string{}

			for endpoint := range diskStatus {
				endpoints = append(endpoints, endpoint)
			}
			sort.Sort(sortorder.Natural(endpoints))

			for _, endpoint := range endpoints {
				disk := diskStatus[endpoint]

				metricBuilder := strings.Builder{}
				builderFn := func(key string, value uint64) {
					if value == 0 {
						return
					}
					if metricBuilder.Len() > 0 {
						metricBuilder.WriteString(", ")
					}
					metricBuilder.WriteString(fmt.Sprintf("%s=%d", key, value))
				}
				metricData := ""
				if disk.Metrics != nil {
					metrics := disk.Metrics
					builderFn("tokens", uint64(metrics.TotalTokens))
					builderFn("write", metrics.TotalWrites)
					builderFn("del", metrics.TotalDeletes)
					builderFn("waiting", uint64(metrics.TotalWaiting))
					builderFn("tout", metrics.TotalErrorsTimeout)
					if metrics.TotalErrorsTimeout != metrics.TotalErrorsAvailability {
						builderFn("err", metrics.TotalErrorsAvailability)
					}

					// if metrics.TotalErrorsTimeout > 0 || metrics.TotalErrorsAvailability > 0 {
					// 	metricData = fmt.Sprintf("wait=%+v, writes=%d", metrics.LastMinute, metrics.TotalWrites)
					// 	if metrics.TotalErrorsTimeout == metrics.TotalErrorsAvailability {
					// 		metricData = fmt.Sprintf("%s, tout=%d", metricData, metrics.TotalErrorsTimeout)
					// 	} else {
					// 		metricData = fmt.Sprintf("%s, tout=%d, err=%d", metricData, metrics.TotalErrorsTimeout, metrics.TotalErrorsAvailability)
					// 	}
					// }
					if metricBuilder.Len() > 0 {
						metricData = fmt.Sprintf("[%s]", metricBuilder.String())
					}
				}

				// disk usage
				diskUsage := ""
				if disk.TotalSpace != 0 && disk.FreeInodes != 0 {
					totalInodes := disk.UsedInodes + disk.FreeInodes
					diskUsage = fmt.Sprintf("disk=%.0f%%[%s], inode=%.0f%% ",
						float64(disk.UsedSpace)/float64(disk.TotalSpace)*100.0,
						humanize.IBytes(disk.TotalSpace),
						float64(disk.UsedInodes)/float64(totalInodes)*100.0,
					)
				}

				fmt.Printf("%s = %s %s%s\n", endpoint, disk.Status, diskUsage, metricData)
				poolStatus, ok := _driveStatus[poolIndex]
				if !ok {
					poolStatus = make(map[string]int)
				}

				_status, ok := poolStatus[disk.Status]
				if !ok {
					_status = 0
				}

				poolStatus[disk.Status] = _status + 1

				_driveStatus[poolIndex] = poolStatus
			}
		}
	}
	// print drive status
	fmt.Printf("\n%+v\n", _driveStatus)
	printOverall(infoStruct)

	// drawTable()

}

func printOverall(infoStruct clusterStruct) {
	// disk raw details
	var rawTotalSize uint64 = 0
	var rawUsedSize uint64 = 0

	noDrives := 0

	for _, server := range infoStruct.Info.Servers {
		for _, disk := range server.Disks {
			// update size
			rawTotalSize += disk.TotalSpace
			rawUsedSize += disk.UsedSpace
			noDrives++
		}
	}

	fmt.Println()
	fmt.Printf("deploymentID=%s\n", infoStruct.Info.DeploymentID)
	fmt.Printf("totalSets=%v, standardSCParity=%d, rrSCParity=%d, totalDriversPerSet=%v\n",
		infoStruct.Info.Backend.TotalSets, infoStruct.Info.Backend.StandardSCParity, infoStruct.Info.Backend.RRSCParity, infoStruct.Info.Backend.DrivesPerSet)
	// print buckets, objects, versions, and deletemarkers
	fmt.Printf("buckets=%d, objects=%d, versions=%d, deletemarkers=%d, usage=%s\n",
		infoStruct.Info.Buckets.Count, infoStruct.Info.Objects.Count, infoStruct.Info.Versions.Count, infoStruct.Info.DeleteMarkers.Count, humanize.IBytes(infoStruct.Info.Usage.Size))
	fmt.Printf("drive_raw_stats: drives=%d, total=%s, used=%s, free=%s\n", noDrives, humanize.IBytes(rawTotalSize), humanize.IBytes(rawUsedSize), humanize.IBytes(rawTotalSize-rawUsedSize))
}

func trimDomainData(endpoint, domainString string) string {
	if domainString == "" {
		return strings.SplitN(endpoint, ".", 2)[0]
	}
	return strings.TrimSuffix(strings.TrimSuffix(endpoint, domainString), ".")
}

func drawTable() {
	app := tview.NewApplication()
	dropdown := tview.NewDropDown().
		SetLabel("Select an option (hit Enter): ").
		SetOptions([]string{"First", "Second", "Third", "Fourth", "Fifth"}, nil)
	if err := app.SetRoot(dropdown, true).SetFocus(dropdown).Run(); err != nil {
		panic(err)
	}

	table := tview.NewTable().
		SetBorders(true)
	lorem := strings.Split("Lorem ipsum dolor sit amet, consetetur sadipscing elitr, sed diam nonumy eirmod tempor invidunt ut labore et dolore magna aliquyam erat, sed diam voluptua. At vero eos et accusam et justo duo dolores et ea rebum. Stet clita kasd gubergren, no sea takimata sanctus est Lorem ipsum dolor sit amet. Lorem ipsum dolor sit amet, consetetur sadipscing elitr, sed diam nonumy eirmod tempor invidunt ut labore et dolore magna aliquyam erat, sed diam voluptua. At vero eos et accusam et justo duo dolores et ea rebum. Stet clita kasd gubergren, no sea takimata sanctus est Lorem ipsum dolor sit amet.", " ")
	cols, rows := 10, 40
	word := 0
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			color := tcell.ColorWhite
			if c < 1 || r < 1 {
				color = tcell.ColorYellow
			}
			table.SetCell(r, c,
				tview.NewTableCell(lorem[word]).
					SetTextColor(color).
					SetAlign(tview.AlignCenter))
			word = (word + 1) % len(lorem)
		}
	}
	table.Select(0, 0).SetFixed(1, 1).SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEscape {
			app.Stop()
		}
		if key == tcell.KeyEnter {
			table.SetSelectable(true, true)
		}
	}).SetSelectedFunc(func(row int, column int) {
		table.GetCell(row, column).SetTextColor(tcell.ColorRed)
		table.SetSelectable(false, false)
	})
	if err := app.SetRoot(table, true).SetFocus(table).Run(); err != nil {
		panic(err)
	}
}

// Source: https://gist.github.com/harshavardhana/327e0577c4fed9211f65
// humanizeDuration humanizes time.Duration output to a meaningful value,
// golang's default “time.Duration“ output is badly formatted and unreadable.
func humanizeDuration(duration time.Duration) string {
	if duration.Seconds() < 60.0 {
		return fmt.Sprintf("%d seconds", int64(duration.Seconds()))
	}
	if duration.Minutes() < 60.0 {
		remainingSeconds := math.Mod(duration.Seconds(), 60)
		return fmt.Sprintf("%d minutes %d seconds", int64(duration.Minutes()), int64(remainingSeconds))
	}
	if duration.Hours() < 24.0 {
		remainingMinutes := math.Mod(duration.Minutes(), 60)
		remainingSeconds := math.Mod(duration.Seconds(), 60)
		return fmt.Sprintf("%d hours %d minutes %d seconds",
			int64(duration.Hours()), int64(remainingMinutes), int64(remainingSeconds))
	}
	remainingHours := math.Mod(duration.Hours(), 24)
	remainingMinutes := math.Mod(duration.Minutes(), 60)
	remainingSeconds := math.Mod(duration.Seconds(), 60)
	return fmt.Sprintf("%d days %d hours %d minutes %d seconds",
		int64(duration.Hours()/24), int64(remainingHours),
		int64(remainingMinutes), int64(remainingSeconds))
}
