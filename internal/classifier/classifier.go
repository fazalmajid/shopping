// Package classifier assigns grocery items to supermarket sections.
// If LLAMA_SERVER_PATH and LLAMA_MODEL_PATH are both configured, it spawns
// a llama-server subprocess and classifies via its HTTP API. Otherwise items
// are assigned to "Other" immediately.
package classifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"shopping/internal/db"
	"shopping/internal/sse"
)

type ClassifyRequest struct {
	ItemID int64
	Text   string
}

type Classifier struct {
	queue   chan ClassifyRequest
	queries *db.Queries
	broker  *sse.Broker

	mu       sync.RWMutex
	sections []db.Section

	wg      sync.WaitGroup
	predict func(text string, sections []db.Section) int
	cmd     *exec.Cmd
	cmdDone chan struct{} // closed by a goroutine that calls cmd.Wait()
}

// New creates a Classifier. If serverBin and modelPath are both non-empty it
// starts llama-server as a subprocess and uses its HTTP API; otherwise it
// falls back to a stub that assigns every item to "Other".
func New(serverBin, modelPath string, queries *db.Queries, broker *sse.Broker, sections []db.Section) (*Classifier, error) {
	c := &Classifier{
		queue:    make(chan ClassifyRequest, 64),
		queries:  queries,
		broker:   broker,
		sections: sections,
	}

	if serverBin == "" || modelPath == "" {
		log.Println("classifier: llama-server not configured; items will be assigned to 'Other'")
		c.predict = func(_ string, secs []db.Section) int { return otherSectionID(secs) }
		c.wg.Add(1)
		go c.worker()
		return c, nil
	}

	port, err := freePort()
	if err != nil {
		return nil, fmt.Errorf("finding free port for llama-server: %w", err)
	}

	cmd := exec.Command(serverBin,
		"--model", modelPath,
		"--host", "127.0.0.1",
		"--port", strconv.Itoa(port),
		"--ctx-size", "512",
		"--threads", "4",
		"--log-disable",
	)
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting llama-server: %w", err)
	}
	c.cmd = cmd
	c.cmdDone = make(chan struct{})
	go func() { cmd.Wait(); close(c.cmdDone) }()

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	log.Printf("classifier: llama-server started (pid %d), waiting for model load…", cmd.Process.Pid)
	if err := waitReady(c.cmdDone, baseURL+"/health", 120*time.Second); err != nil {
		cmd.Process.Kill()
		<-c.cmdDone
		return nil, fmt.Errorf("llama-server did not become ready: %w", err)
	}
	log.Println("classifier: llama-server ready")

	c.predict = func(text string, secs []db.Section) int {
		return predictHTTP(baseURL, text, secs)
	}
	c.wg.Add(1)
	go c.worker()
	return c, nil
}

func (c *Classifier) Stop() {
	close(c.queue)
	c.wg.Wait()
	if c.cmd != nil {
		c.cmd.Process.Kill()
		<-c.cmdDone // wait for the cmd.Wait() goroutine; avoids double-Wait
	}
}

// Enqueue submits an item for async classification. Drops the request if the
// queue is full so the broker goroutine is never blocked.
func (c *Classifier) Enqueue(req ClassifyRequest) {
	select {
	case c.queue <- req:
	default:
		log.Printf("classifier: queue full, dropping item %d", req.ItemID)
	}
}

// ReloadSections appends a newly created section so future prompts include it.
func (c *Classifier) ReloadSections(s db.Section) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sections = append(c.sections, s)
}

func (c *Classifier) worker() {
	defer c.wg.Done()
	for req := range c.queue {
		c.mu.RLock()
		sections := c.sections
		c.mu.RUnlock()

		sectionID := c.predict(req.Text, sections)
		if sectionID == 0 {
			continue
		}

		ctx := context.Background()
		if err := c.queries.UpdateItemSection(ctx, req.ItemID, sectionID); err != nil {
			log.Printf("classifier: update item %d: %v", req.ItemID, err)
			continue
		}
		c.queries.UpsertItemSection(ctx, req.Text, sectionID, "llm")
		c.broker.Publish(sse.Event{
			Type: "item_classified",
			Data: map[string]any{"id": req.ItemID, "section_id": sectionID},
		})
	}
}

type completionReq struct {
	Prompt      string   `json:"prompt"`
	NPredict    int      `json:"n_predict"`
	Temperature float64  `json:"temperature"`
	Stop        []string `json:"stop"`
}

type completionResp struct {
	Content string `json:"content"`
}

func predictHTTP(baseURL, text string, sections []db.Section) int {
	if len(sections) == 0 {
		return 0
	}
	var sb strings.Builder
	for i, s := range sections {
		fmt.Fprintf(&sb, "%d. %s\n", i+1, s.Name)
	}
	prompt := fmt.Sprintf(
		"You are a grocery store item categorizer.\n"+
			"Sections:\n%s\n"+
			"Which section number does \"%s\" belong to? Reply with only the number.\n",
		sb.String(), text,
	)

	body, _ := json.Marshal(completionReq{
		Prompt:      prompt,
		NPredict:    4,
		Temperature: 0,
		Stop:        []string{"\n"},
	})
	resp, err := http.Post(baseURL+"/completion", "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("classifier: completion request: %v", err)
		return otherSectionID(sections)
	}
	defer resp.Body.Close()

	var cr completionResp
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		log.Printf("classifier: decode response: %v", err)
		return otherSectionID(sections)
	}

	n, err := strconv.Atoi(strings.TrimSpace(cr.Content))
	if err != nil || n < 1 || n > len(sections) {
		return otherSectionID(sections)
	}
	return sections[n-1].ID
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func waitReady(dead <-chan struct{}, healthURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-dead:
			return fmt.Errorf("process exited unexpectedly")
		default:
		}
		resp, err := http.Get(healthURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timed out after %s", timeout)
}

func otherSectionID(sections []db.Section) int {
	for _, s := range sections {
		if s.Name == "Other" {
			return s.ID
		}
	}
	if len(sections) > 0 {
		return sections[len(sections)-1].ID
	}
	return 0
}
