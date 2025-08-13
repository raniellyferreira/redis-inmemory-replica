package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DatabaseStats represents the statistics for a single database
type DatabaseStats struct {
	Keys    int64
	Expires int64
	AvgTTL  int64 // in milliseconds, 0 if not present
}

// KeyspaceInfo represents the complete keyspace information
type KeyspaceInfo map[int]DatabaseStats

func main() {
	var refAddr = flag.String("ref", "", "Reference Redis endpoint (host:port)")
	var sutAddr = flag.String("sut", "", "System under test Redis endpoint (host:port)")
	var dbsFlag = flag.String("dbs", "", "Comma-separated list of database numbers to compare (e.g., 8,9,11,13)")
	var helpFlag = flag.Bool("help", false, "Show help message")

	flag.Parse()

	if *helpFlag || *refAddr == "" || *sutAddr == "" {
		fmt.Println("INFO Keyspace Comparison Tool")
		fmt.Println("=============================")
		fmt.Println("Usage: info-diff --ref=host:port --sut=host:port [--dbs=8,9,11,13]")
		fmt.Println("")
		fmt.Println("Flags:")
		fmt.Println("  --ref     Reference Redis endpoint (e.g., localhost:6379)")
		fmt.Println("  --sut     System under test Redis endpoint (e.g., localhost:6380)")
		fmt.Println("  --dbs     Optional: Specific database numbers to compare")
		fmt.Println("  --help    Show this help message")
		fmt.Println("")
		fmt.Println("Example:")
		fmt.Println("  info-diff --ref=localhost:6379 --sut=localhost:6380 --dbs=8,9,11,13")
		os.Exit(0)
	}

	// Parse database filter if provided
	var dbFilter map[int]bool
	if *dbsFlag != "" {
		dbFilter = make(map[int]bool)
		for _, dbStr := range strings.Split(*dbsFlag, ",") {
			if db, err := strconv.Atoi(strings.TrimSpace(dbStr)); err == nil {
				dbFilter[db] = true
			}
		}
	}

	fmt.Printf("Comparing keyspace information:\n")
	fmt.Printf("  Reference: %s\n", *refAddr)
	fmt.Printf("  System:    %s\n", *sutAddr)
	if dbFilter != nil {
		fmt.Printf("  Databases: %s\n", *dbsFlag)
	}
	fmt.Println()

	// Get keyspace info from both endpoints
	refInfo, err := getKeyspaceInfo(*refAddr)
	if err != nil {
		log.Fatalf("Failed to get keyspace info from reference %s: %v", *refAddr, err)
	}

	sutInfo, err := getKeyspaceInfo(*sutAddr)
	if err != nil {
		log.Fatalf("Failed to get keyspace info from system %s: %v", *sutAddr, err)
	}

	// Compare and display differences
	compareKeyspaceInfo(refInfo, sutInfo, dbFilter)
}

// getKeyspaceInfo connects to a Redis endpoint and extracts keyspace information
func getKeyspaceInfo(addr string) (KeyspaceInfo, error) {
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Close()

	// Send INFO keyspace command
	_, err = conn.Write([]byte("*2\r\n$4\r\nINFO\r\n$8\r\nkeyspace\r\n"))
	if err != nil {
		return nil, fmt.Errorf("failed to send command: %w", err)
	}

	// Read response
	scanner := bufio.NewScanner(conn)
	var response strings.Builder
	
	// Read the response length line first
	if !scanner.Scan() {
		return nil, fmt.Errorf("failed to read response")
	}
	
	// Read the response content
	if !scanner.Scan() {
		return nil, fmt.Errorf("failed to read response content")
	}
	
	response.WriteString(scanner.Text())

	return parseKeyspaceInfo(response.String())
}

// parseKeyspaceInfo extracts keyspace information from INFO response
func parseKeyspaceInfo(infoResponse string) (KeyspaceInfo, error) {
	keyspace := make(KeyspaceInfo)
	
	// Regular expression to match database lines like: db0:keys=2,expires=0,avg_ttl=0
	dbRegex := regexp.MustCompile(`db(\d+):keys=(\d+),expires=(\d+)(?:,avg_ttl=(\d+))?`)
	
	lines := strings.Split(infoResponse, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if matches := dbRegex.FindStringSubmatch(line); matches != nil {
			dbNum, _ := strconv.Atoi(matches[1])
			keys, _ := strconv.ParseInt(matches[2], 10, 64)
			expires, _ := strconv.ParseInt(matches[3], 10, 64)
			
			var avgTTL int64 = 0
			if len(matches) > 4 && matches[4] != "" {
				avgTTL, _ = strconv.ParseInt(matches[4], 10, 64)
			}
			
			keyspace[dbNum] = DatabaseStats{
				Keys:    keys,
				Expires: expires,
				AvgTTL:  avgTTL,
			}
		}
	}
	
	return keyspace, nil
}

// compareKeyspaceInfo compares two keyspace info maps and displays differences
func compareKeyspaceInfo(ref, sut KeyspaceInfo, dbFilter map[int]bool) {
	// Get all database numbers from both systems
	allDBs := make(map[int]bool)
	for db := range ref {
		if dbFilter == nil || dbFilter[db] {
			allDBs[db] = true
		}
	}
	for db := range sut {
		if dbFilter == nil || dbFilter[db] {
			allDBs[db] = true
		}
	}

	// Convert to sorted slice
	var dbNums []int
	for db := range allDBs {
		dbNums = append(dbNums, db)
	}
	sort.Ints(dbNums)

	fmt.Println("Database Comparison Results:")
	fmt.Println("============================")

	differences := 0
	
	for _, dbNum := range dbNums {
		refStats, refExists := ref[dbNum]
		sutStats, sutExists := sut[dbNum]

		if !refExists && !sutExists {
			continue
		}

		fmt.Printf("db%d:\n", dbNum)

		if !refExists {
			fmt.Printf("  ❌ Missing in REFERENCE, present in SYSTEM: keys=%d,expires=%d,avg_ttl=%d\n", 
				sutStats.Keys, sutStats.Expires, sutStats.AvgTTL)
			differences++
		} else if !sutExists {
			fmt.Printf("  ❌ Missing in SYSTEM, present in REFERENCE: keys=%d,expires=%d,avg_ttl=%d\n", 
				refStats.Keys, refStats.Expires, refStats.AvgTTL)
			differences++
		} else {
			// Both exist, compare values
			if refStats.Keys != sutStats.Keys {
				fmt.Printf("  ❌ Keys differ: REF=%d, SUT=%d\n", refStats.Keys, sutStats.Keys)
				differences++
			}
			if refStats.Expires != sutStats.Expires {
				fmt.Printf("  ❌ Expires differ: REF=%d, SUT=%d\n", refStats.Expires, sutStats.Expires)
				differences++
			}
			if refStats.AvgTTL != sutStats.AvgTTL {
				fmt.Printf("  ⚠️  AvgTTL differs: REF=%d, SUT=%d (may be acceptable)\n", refStats.AvgTTL, sutStats.AvgTTL)
				// Don't count avgTTL differences as errors since they can vary
			}
			
			if refStats.Keys == sutStats.Keys && refStats.Expires == sutStats.Expires {
				fmt.Printf("  ✅ Match: keys=%d,expires=%d,avg_ttl=%d/%d\n", 
					refStats.Keys, refStats.Expires, refStats.AvgTTL, sutStats.AvgTTL)
			}
		}
	}

	fmt.Println()
	if differences == 0 {
		fmt.Println("🎉 SUCCESS: No critical differences found!")
	} else {
		fmt.Printf("❌ FAILURE: %d critical differences found\n", differences)
		os.Exit(1)
	}
}