package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var (
	channelsJSON    bool
	channelsTimeout int
)

// ChannelsCommand returns the channels command
func ChannelsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "channels",
		Short: "Manage chat channels",
		Long:  `List and manage chat channels like Telegram, Feishu, WhatsApp, etc.`,
	}

	// Add list subcommand
	cmd.AddCommand(channelsListCmd())

	// Add status subcommand
	cmd.AddCommand(channelsStatusCmd())

	return cmd
}

// ChannelInfo represents information about a channel
type ChannelInfo struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// ChannelStatusResponse represents the response from gateway channels.status
type ChannelStatusResponse struct {
	Name    string                 `json:"name"`
	Enabled bool                   `json:"enabled"`
	Extra   map[string]interface{} `json:"extra,omitempty"`
}

// channelsListCmd returns the channels list command
func channelsListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all available channels",
		Long:  `Display a list of all configured channels and their status.`,
		Run:   runChannelsList,
	}

	cmd.Flags().BoolVarP(&channelsJSON, "json", "j", false, "Output as JSON")
	cmd.Flags().IntVarP(&channelsTimeout, "timeout", "t", 5, "Timeout in seconds")

	return cmd
}

// channelsStatusCmd returns the channels status command
func channelsStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status [channel]",
		Short: "Show channel status",
		Long:  `Display detailed status information for a specific channel or all channels.`,
		Args:  cobra.MaximumNArgs(1),
		Run:   runChannelsStatus,
	}

	cmd.Flags().BoolVarP(&channelsJSON, "json", "j", false, "Output as JSON")
	cmd.Flags().IntVarP(&channelsTimeout, "timeout", "t", 5, "Timeout in seconds")

	return cmd
}

// runChannelsList executes the channels list command
func runChannelsList(cmd *cobra.Command, args []string) {
	// Try to get channel list from gateway
	channels := getChannelsFromGateway(channelsTimeout)

	// Also show known supported channels
	allChannels := getAllKnownChannels()

	if channelsJSON {
		outputChannelsJSON(channels, allChannels)
	} else {
		outputChannelsText(channels, allChannels)
	}
}

// runChannelsStatus executes the channels status command
func runChannelsStatus(cmd *cobra.Command, args []string) {
	channelName := ""
	if len(args) > 0 {
		channelName = args[0]
	}

	status := getChannelStatusFromGateway(channelName, channelsTimeout)

	if channelsJSON {
		outputChannelStatusJSON(status)
	} else {
		outputChannelStatusText(channelName, status)
	}
}

// getAllKnownChannels returns all known supported channel types
func getAllKnownChannels() []ChannelInfo {
	return []ChannelInfo{
		{Name: "feishu", Enabled: false},
		{Name: "telegram", Enabled: false},
		{Name: "whatsapp", Enabled: false},
		{Name: "qq", Enabled: false},
		{Name: "wework", Enabled: false},
		{Name: "dingtalk", Enabled: false},
		{Name: "slack", Enabled: false},
		{Name: "discord", Enabled: false},
		{Name: "teams", Enabled: false},
		{Name: "googlechat", Enabled: false},
	}
}

// getChannelsFromGateway retrieves channel list from gateway
func getChannelsFromGateway(timeout int) []ChannelInfo {
	client := &http.Client{
		Timeout: time.Duration(timeout) * time.Second,
	}

	// Try different WebSocket gateway ports
	ports := []int{18789, 18790, 18890}
	var channels []ChannelInfo

	for _, port := range ports {
		// Use the gateway's HTTP health endpoint to check if it's running
		// Note: The actual channel list comes from the WebSocket interface
		url := fmt.Sprintf("http://localhost:%d/health", port)
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			// Gateway is running, so we can show configured channels
			// The actual channel status would need to come from the gateway's internal state
			// For now, we'll return empty and let the text output show all known channels
			break
		}
	}

	return channels
}

// getChannelStatusFromGateway retrieves channel status from gateway
func getChannelStatusFromGateway(channelName string, timeout int) map[string]interface{} {
	client := &http.Client{
		Timeout: time.Duration(timeout) * time.Second,
	}

	// Try different WebSocket gateway ports
	ports := []int{18789, 18790, 18890}

	for _, port := range ports {
		url := fmt.Sprintf("http://localhost:%d/health", port)
		resp, err := client.Get(url)
		if err == nil {
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			var health map[string]interface{}
			_ = json.Unmarshal(body, &health)

			// Gateway is online
			return map[string]interface{}{
				"online":      true,
				"gateway_url": url,
				"channel":     channelName,
				"status":      "available",
			}
		}
	}

	// Gateway is offline
	return map[string]interface{}{
		"online":      false,
		"channel":     channelName,
		"status":      "unavailable",
		"message":     "Gateway is not running. Start with 'goclaw start' or 'goclaw gateway run'",
	}
}

// outputChannelsJSON outputs channel list as JSON
func outputChannelsJSON(activeChannels []ChannelInfo, allChannels []ChannelInfo) {
	type output struct {
		Active  []ChannelInfo `json:"active"`
		All     []ChannelInfo `json:"all"`
		Online  bool          `json:"gateway_online"`
		Message string        `json:"message,omitempty"`
	}

	// Check if gateway is online
	gatewayOnline := len(activeChannels) > 0 || checkGatewayOnline(channelsTimeout)

	out := output{
		Active:  activeChannels,
		All:     allChannels,
		Online:  gatewayOnline,
		Message: "Use 'goclaw start' to start the gateway with configured channels",
	}

	jsonOut, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(jsonOut))
}

// outputChannelsText outputs channel list as text
func outputChannelsText(activeChannels []ChannelInfo, allChannels []ChannelInfo) {
	fmt.Println("=== Channels ===")

	// Check gateway status
	gatewayOnline := checkGatewayOnline(channelsTimeout)
	if gatewayOnline {
		fmt.Println("Gateway: Online")
	} else {
		fmt.Println("Gateway: Offline (start with 'goclaw start')")
	}

	fmt.Println("\nAvailable Channels:")
	for _, ch := range allChannels {
		fmt.Printf("  - %s\n", ch.Name)
	}

	fmt.Println("\nTip:")
	fmt.Println("  1. Edit ~/.goclaw/config.json to configure channels")
	fmt.Println("  2. Run 'goclaw start' to start the agent with channels enabled")
	fmt.Println("  3. Use 'goclaw channels status [name]' to check specific channel status")
}

// outputChannelStatusJSON outputs channel status as JSON
func outputChannelStatusJSON(status map[string]interface{}) {
	jsonOut, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(jsonOut))
}

// outputChannelStatusText outputs channel status as text
func outputChannelStatusText(channelName string, status map[string]interface{}) {
	online, _ := status["online"].(bool)

	fmt.Printf("=== Channel Status")
	if channelName != "" {
		fmt.Printf(" (%s)", channelName)
	}
	fmt.Println(" ===")

	if online {
		fmt.Println("Gateway: Online")
		if url, ok := status["gateway_url"].(string); ok {
			fmt.Printf("URL:     %s\n", url)
		}
		fmt.Println("Status:  Available")
	} else {
		fmt.Println("Gateway: Offline")
		fmt.Println("Status:  Unavailable")
		if msg, ok := status["message"].(string); ok {
			fmt.Printf("Message: %s\n", msg)
		}
	}
}

// checkGatewayOnline checks if the gateway is running
func checkGatewayOnline(timeout int) bool {
	client := &http.Client{
		Timeout: time.Duration(timeout) * time.Second,
	}

	ports := []int{18789, 18790, 18890}

	for _, port := range ports {
		url := fmt.Sprintf("http://localhost:%d/health", port)
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
	}

	return false
}
