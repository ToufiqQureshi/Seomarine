package ranktracking

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
)

// TaskInput is one keyword and device to queue.
type TaskInput struct {
	KeywordID string
	Keyword   string
	Device    string
}

// PostRequest queues up to MaxTasksPerPost lookups that share a market.
type PostRequest struct {
	Tasks        []TaskInput
	LocationCode int
	LanguageCode string
	LocationName *string
	Depth        int
	TargetDomain string
}

// PostedTask is a lookup the provider accepted, and has charged for.
type PostedTask struct {
	TaskInput
	TaskID string
}

// TaskOutcome is the state of a queued task.
type TaskOutcome struct {
	Pending bool
	Failed  string // the provider's message when the task failed
	Result  CheckResult
}

// QueuedSerp runs SERP lookups through the provider's task queue, which costs
// about 30% of the live endpoint. Tasks finish in a few minutes.
type QueuedSerp interface {
	// PostTasks queues the tasks and returns those the provider accepted. A
	// rejected task is simply missing from the result.
	PostTasks(ctx context.Context, organizationID string, req PostRequest) ([]PostedTask, error)
	// CollectTask reads one task. It is free: the task was charged when posted.
	CollectTask(ctx context.Context, taskID, targetDomain string) (TaskOutcome, error)
}

const (
	taskPostPath = "/v3/serp/google/organic/task_post"
	taskGetPath  = "/v3/serp/google/organic/task_get/advanced/"
	taskCreated  = 20100
)

// Statuses of a task that has not finished yet.
var inProgressStatuses = []int{20100, 40601, 40602}

// PostTasks queues the lookups on DataForSEO's standard-priority queue.
func (p DataForSEOSerp) PostTasks(ctx context.Context, organizationID string, req PostRequest) ([]PostedTask, error) {
	if len(req.Tasks) == 0 || len(req.Tasks) > MaxTasksPerPost {
		return nil, fmt.Errorf("task_post accepts 1-%d tasks, got %d", MaxTasksPerPost, len(req.Tasks))
	}
	if req.Depth < 10 || req.Depth > 100 || req.Depth%10 != 0 {
		return nil, fmt.Errorf("rank check depth %d is not a provider depth", req.Depth)
	}
	byTag := make(map[string]TaskInput, len(req.Tasks))
	body := make([]map[string]any, len(req.Tasks))
	for i, t := range req.Tasks {
		tag := t.KeywordID + ":" + t.Device
		byTag[tag] = t
		body[i] = map[string]any{
			"keyword":       t.Keyword,
			"language_code": req.LanguageCode,
			"device":        t.Device,
			"os":            map[string]string{"desktop": "windows", "mobile": "android"}[t.Device],
			"depth":         req.Depth,
			"stop_crawl_on_match": []map[string]string{
				{"match_value": req.TargetDomain, "match_type": "with_subdomains"},
			},
			"find_targets_in": []string{"organic"},
			// Echoed on the response entry: how a task id maps back to a keyword
			// without relying on order.
			"tag": tag,
		}
		if req.LocationName != nil && *req.LocationName != "" {
			body[i]["location_name"] = *req.LocationName
		} else {
			body[i]["location_code"] = req.LocationCode
		}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode task_post request: %w", err)
	}
	// Not retry-safe: posting twice would charge twice.
	response, err := p.Client.Do(ctx, organizationID, http.MethodPost, taskPostPath, raw, false)
	if err != nil {
		return nil, fmt.Errorf("DataForSEO %s: %w", taskPostPath, err)
	}
	var envelope struct {
		StatusCode    int    `json:"status_code"`
		StatusMessage string `json:"status_message"`
		Tasks         []struct {
			ID         string `json:"id"`
			StatusCode int    `json:"status_code"`
			Data       struct {
				Tag string `json:"tag"`
			} `json:"data"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(response, &envelope); err != nil {
		return nil, fmt.Errorf("decode task_post response: %w", err)
	}
	if envelope.StatusCode != 20000 {
		return nil, fmt.Errorf("DataForSEO %s: %w", taskPostPath, classifyStatus(envelope.StatusCode, envelope.StatusMessage))
	}
	// One entry per submitted task; accepted ones are "Task Created" with their
	// own cost, already recorded by the client for every entry.
	var posted []PostedTask
	for _, entry := range envelope.Tasks {
		input, known := byTag[entry.Data.Tag]
		if entry.StatusCode != taskCreated || entry.ID == "" || !known {
			continue
		}
		posted = append(posted, PostedTask{TaskInput: input, TaskID: entry.ID})
	}
	return posted, nil
}

// CollectTask fetches a queued task. The call goes unmetered because the
// response repeats the settled cost of a task that was charged at post time.
func (p DataForSEOSerp) CollectTask(ctx context.Context, taskID, targetDomain string) (TaskOutcome, error) {
	response, err := p.Client.DoUnmetered(ctx, http.MethodGet, taskGetPath+url.PathEscape(taskID), nil, true)
	if err != nil {
		return TaskOutcome{}, fmt.Errorf("DataForSEO task_get: %w", err)
	}
	var envelope struct {
		StatusCode    int    `json:"status_code"`
		StatusMessage string `json:"status_message"`
		Tasks         []struct {
			StatusCode    int               `json:"status_code"`
			StatusMessage string            `json:"status_message"`
			Result        []json.RawMessage `json:"result"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(response, &envelope); err != nil {
		return TaskOutcome{}, fmt.Errorf("decode task_get response: %w", err)
	}
	if envelope.StatusCode != 20000 || len(envelope.Tasks) == 0 {
		return TaskOutcome{}, fmt.Errorf("DataForSEO task_get failed: %s", envelope.StatusMessage)
	}
	task := envelope.Tasks[0]
	for _, status := range inProgressStatuses {
		if task.StatusCode == status {
			return TaskOutcome{Pending: true}, nil
		}
	}
	if task.StatusCode != 20000 {
		// "No Search Results" is a valid empty SERP for a new or obscure keyword.
		if strings.Contains(strings.ToLower(task.StatusMessage), "no search results") {
			return TaskOutcome{Result: CheckResult{SerpFeatures: []string{}}}, nil
		}
		message := task.StatusMessage
		if message == "" {
			message = fmt.Sprintf("DataForSEO task failed (%d)", task.StatusCode)
		}
		return TaskOutcome{Failed: message}, nil
	}
	result, err := parseSerp(task.Result, targetDomain)
	if err != nil {
		return TaskOutcome{}, err
	}
	return TaskOutcome{Result: result}, nil
}

// classifyStatus maps a whole-request failure to the shared provider errors.
func classifyStatus(status int, message string) error {
	body, _ := json.Marshal(map[string]any{"status_code": status, "status_message": message})
	_, err := dataforseo.Results(body)
	if err == nil {
		err = errors.New(message)
	}
	return err
}
