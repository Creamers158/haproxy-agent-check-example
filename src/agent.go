package main

import (
        "bytes"
        "encoding/json"
        "fmt"
        "io/ioutil"
        "math/big"
        "net/http"
        "time"
        "github.com/firstrow/tcp_server"
        "bufio"
        "strconv"
        "net"
)

const (
        checkHost = "127.0.0.1" // Replace with actual CHECK_HOST
        checkPort = "8545"       // Replace with actual CHECK_PORT
)

var threshold int = 60 // Default threshold in case HAProxy doesn't send any value

func main() {
                server := tcp_server.New(":3000")

                server.OnNewClient(func(c *tcp_server.Client) {
                fmt.Println("HAProxy connected to health check agent")

                // Fetch and parse the threshold sent by HAProxy
                receivedThreshold, err := fetchThresholdFromHAProxy(c.Conn())
                if err == nil {
                        threshold = receivedThreshold
                        fmt.Printf("Received threshold from HAProxy: %d sec\n", threshold)
                } else {
                        fmt.Println("Error reading threshold from HAProxy, using default:", err)
                }

                //getBlockage
                blockage, err := getBlockage()
                if err != nil {
                        fmt.Println(err)
                        c.Close()
                        return
                }

                if blockage > int64(threshold) {
                        fmt.Printf("\033[1;31mBlockage time difference: %d seconds - Setting server weight to 50%%\033[0m\n", blockage)
                        c.Send("50%\n")
                } else {
                        fmt.Printf("Blockage time difference (INFO): %d sec (Threshold %d sec) - Setting server weight to 100%%\n", blockage, threshold)
                        c.Send("100%\n")
                }

                c.Close()
        })

        server.Listen()
}

func fetchThresholdFromHAProxy(conn net.Conn) (int, error) {
        // Read data sent by HAProxy on connection
        reader := bufio.NewReader(conn)
        data, err := reader.ReadBytes('\n') // HAProxy sends data ending with newline
        if err != nil {
                return 0, fmt.Errorf("failed to read from client: %w", err)
        }

        // Trim and parse the received data as integer
        data = bytes.TrimSpace(data)
        thresholdValue, err := strconv.Atoi(string(data))
        if err != nil {
                return 0, fmt.Errorf("failed to parse threshold value: %w", err)
        }

        return thresholdValue, nil
}

func getBlockage() (int64, error) {
        // Step 1: Fetch the hex timestamp from the API
        hexTimestamp, err := fetchHexTimestamp()
        if err != nil {
                return 0, fmt.Errorf("failed to fetch hex timestamp: %w", err)
        }

        // Step 2: Convert hex timestamp to epoch time (int64)
        epochTimestamp, err := hexToEpoch(hexTimestamp)
        if err != nil {
                return 0, fmt.Errorf("failed to convert hex to epoch: %w", err)
        }

        // Step 3: Calculate the difference from the current time
        currentTimestamp := time.Now().Unix()
        blockage := currentTimestamp - epochTimestamp
        return blockage, nil
}

func fetchHexTimestamp() (string, error) {
        url := fmt.Sprintf("http://%s:%s", checkHost, checkPort)
        payload := map[string]interface{}{
                "jsonrpc": "2.0",
                "method":  "eth_getBlockByNumber",
                "params":  []interface{}{"latest", false},
                "id":      1,
        }

        payloadBytes, err := json.Marshal(payload)
        if err != nil {
                return "", fmt.Errorf("failed to marshal payload: %w", err)
        }

        resp, err := http.Post(url, "application/json", bytes.NewBuffer(payloadBytes))
        if err != nil {
                return "", fmt.Errorf("HTTP request failed: %w", err)
        }
        defer resp.Body.Close()

        body, err := ioutil.ReadAll(resp.Body)
        if err != nil {
                return "", fmt.Errorf("failed to read response body: %w", err)
        }

        // Parsing JSON response to extract the hex timestamp
        var response map[string]interface{}
        if err := json.Unmarshal(body, &response); err != nil {
                return "", fmt.Errorf("failed to parse JSON response: %w", err)
        }

        result, ok := response["result"].(map[string]interface{})
        if !ok || result["timestamp"] == nil {
                return "", fmt.Errorf("timestamp not found in response")
        }

        hexTimestamp, ok := result["timestamp"].(string)
        if !ok {
                return "", fmt.Errorf("timestamp is not a string")
        }

        return hexTimestamp, nil
}

func hexToEpoch(hexTimestamp string) (int64, error) {
        // Remove "0x" prefix if present
        if len(hexTimestamp) > 1 && hexTimestamp[:2] == "0x" {
                hexTimestamp = hexTimestamp[2:]
        }

        // Convert the hex string to an integer
        epochTimestamp, success := new(big.Int).SetString(hexTimestamp, 16)
        if !success {
                return 0, fmt.Errorf("failed to parse hex timestamp: %s", hexTimestamp)
        }

        return epochTimestamp.Int64(), nil
}
