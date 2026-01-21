package admin

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// BatchRegisterConfig 批量注册配置
type BatchRegisterConfig struct {
	Count             int                    `json:"count"`
	WorkerDomain      string                 `json:"worker_domain"`
	AdminToken        string                 `json:"admin_token"`
	CRSAPIBase        string                 `json:"crs_api_base"`
	CRSAdminToken     string                 `json:"crs_admin_token"`
	EnableTeamInvite  bool                   `json:"enable_team_invite"`
	Teams             []TeamConfig           `json:"teams,omitempty"`
	OpenAIConfig      *OpenAIRegisterConfig  `json:"openai_config,omitempty"`
}

// OpenAIRegisterConfig OpenAI注册配置
type OpenAIRegisterConfig struct {
	TargetURL       string `json:"target_url"`
	DefaultName     string `json:"default_name"`
	DefaultBirthday string `json:"default_birthday"`
}

// AccountResult 账号注册结果
type AccountResult struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
}

// BatchRegisterTask 批量注册任务
type BatchRegisterTask struct {
	ID            string          `json:"id"`
	Status        string          `json:"status"` // "running", "completed", "failed", "cancelled"
	TotalCount    int             `json:"total_count"`
	CurrentIndex  int             `json:"current_index"`
	CurrentStatus string          `json:"current_status"`
	Logs          []string        `json:"logs"`
	CreatedAt     time.Time       `json:"created_at"`
	CompletedAt   *time.Time      `json:"completed_at,omitempty"`
	Results       []AccountResult `json:"results"`
	mu            sync.RWMutex
}

// AddLog 添加日志
func (t *BatchRegisterTask) AddLog(msg string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Logs = append(t.Logs, msg)
}

// UpdateStatus 更新状态
func (t *BatchRegisterTask) UpdateStatus(status string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Status = status
	if status == "completed" || status == "failed" || status == "cancelled" {
		now := time.Now()
		t.CompletedAt = &now
	}
}

// UpdateProgress 更新进度
func (t *BatchRegisterTask) UpdateProgress(currentIndex int, currentStatus string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.CurrentIndex = currentIndex
	t.CurrentStatus = currentStatus
}

// AddResult 添加结果
func (t *BatchRegisterTask) AddResult(result AccountResult) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Results = append(t.Results, result)
}

// GetSnapshot 获取任务快照（用于JSON序列化）
func (t *BatchRegisterTask) GetSnapshot() map[string]interface{} {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return map[string]interface{}{
		"id":             t.ID,
		"status":         t.Status,
		"total_count":    t.TotalCount,
		"current_index":  t.CurrentIndex,
		"current_status": t.CurrentStatus,
		"created_at":     t.CreatedAt,
		"completed_at":   t.CompletedAt,
		"results_count":  len(t.Results),
	}
}

// GetFullSnapshot 获取完整快照（包含日志和结果）
func (t *BatchRegisterTask) GetFullSnapshot() map[string]interface{} {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return map[string]interface{}{
		"id":             t.ID,
		"status":         t.Status,
		"total_count":    t.TotalCount,
		"current_index":  t.CurrentIndex,
		"current_status": t.CurrentStatus,
		"logs":           t.Logs,
		"created_at":     t.CreatedAt,
		"completed_at":   t.CompletedAt,
		"results":        t.Results,
	}
}

// TaskManager 任务管理器
type TaskManager struct {
	tasks   map[string]*BatchRegisterTask
	mu      sync.RWMutex
	maxTasks int
	ttl     time.Duration
}

// NewTaskManager 创建任务管理器
func NewTaskManager(maxTasks int, ttl time.Duration) *TaskManager {
	tm := &TaskManager{
		tasks:    make(map[string]*BatchRegisterTask),
		maxTasks: maxTasks,
		ttl:      ttl,
	}
	// 启动定期清理
	go tm.cleanupExpiredTasks()
	return tm
}

// CreateTask 创建任务
func (tm *TaskManager) CreateTask(config BatchRegisterConfig) (*BatchRegisterTask, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// 检查是否超过最大任务数
	runningCount := 0
	for _, task := range tm.tasks {
		task.mu.RLock()
		if task.Status == "running" {
			runningCount++
		}
		task.mu.RUnlock()
	}

	if runningCount >= tm.maxTasks {
		return nil, fmt.Errorf("已达到最大并发任务数 (%d)，请稍后再试", tm.maxTasks)
	}

	// 生成任务ID
	taskID := fmt.Sprintf("task_%s_%06d", time.Now().Format("20060102150405"), rand.Intn(1000000))

	task := &BatchRegisterTask{
		ID:            taskID,
		Status:        "running",
		TotalCount:    config.Count,
		CurrentIndex:  0,
		CurrentStatus: "初始化...",
		Logs:          []string{},
		CreatedAt:     time.Now(),
		Results:       []AccountResult{},
	}

	tm.tasks[taskID] = task
	return task, nil
}

// GetTask 获取任务
func (tm *TaskManager) GetTask(taskID string) (*BatchRegisterTask, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	task, ok := tm.tasks[taskID]
	return task, ok
}

// DeleteTask 删除任务
func (tm *TaskManager) DeleteTask(taskID string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	delete(tm.tasks, taskID)
}

// cleanupExpiredTasks 定期清理过期任务
func (tm *TaskManager) cleanupExpiredTasks() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		tm.mu.Lock()
		now := time.Now()
		for taskID, task := range tm.tasks {
			task.mu.RLock()
			shouldDelete := false
			if task.CompletedAt != nil {
				// 已完成的任务，检查是否超过TTL
				if now.Sub(*task.CompletedAt) > tm.ttl {
					shouldDelete = true
				}
			}
			task.mu.RUnlock()

			if shouldDelete {
				delete(tm.tasks, taskID)
			}
		}
		tm.mu.Unlock()
	}
}

