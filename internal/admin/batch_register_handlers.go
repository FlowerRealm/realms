package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// StartBatchRegister 启动批量注册
func (s *Server) StartBatchRegister(w http.ResponseWriter, r *http.Request) {
	// 解析请求
	var config BatchRegisterConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		http.Error(w, fmt.Sprintf("解析请求失败: %v", err), http.StatusBadRequest)
		return
	}

	// 验证参数
	if config.Count < 1 || config.Count > 10 {
		http.Error(w, "注册数量必须在1-10之间", http.StatusBadRequest)
		return
	}

	if config.WorkerDomain == "" || config.AdminToken == "" {
		http.Error(w, "Worker Domain 和 Admin Token 不能为空", http.StatusBadRequest)
		return
	}

	// 创建任务
	task, err := s.batchRegisterTaskManager.CreateTask(config)
	if err != nil {
		http.Error(w, err.Error(), http.StatusTooManyRequests)
		return
	}

	slog.Info("批量注册任务已创建", "task_id", task.ID, "count", config.Count)

	// 启动异步执行
	go func() {
		ctx := context.Background()
		if err := s.goRegisterExecutor.Execute(ctx, task, config); err != nil {
			task.UpdateStatus("failed")
			task.AddLog(fmt.Sprintf("❌ 执行失败: %v", err))
			slog.Error("批量注册任务失败", "task_id", task.ID, "error", err)
		} else {
			task.UpdateStatus("completed")
			task.AddLog("🎉 所有任务已完成")
			slog.Info("批量注册任务完成", "task_id", task.ID)
		}
	}()

	// 返回任务ID
	response := map[string]interface{}{
		"task_id":      task.ID,
		"status":       "started",
		"progress_url": fmt.Sprintf("/admin/batch-register-progress/%s", task.ID),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// StreamBatchRegisterProgress SSE流式推送进度
func (s *Server) StreamBatchRegisterProgress(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("task_id")
	if taskID == "" {
		http.Error(w, "任务ID不能为空", http.StatusBadRequest)
		return
	}

	// 获取任务
	task, ok := s.batchRegisterTaskManager.GetTask(taskID)
	if !ok {
		http.Error(w, "任务不存在", http.StatusNotFound)
		return
	}

	// 设置SSE头
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // 禁用nginx缓冲

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "不支持流式响应", http.StatusInternalServerError)
		return
	}

	slog.Info("SSE连接已建立", "task_id", taskID)

	// 发送当前进度
	lastLogIndex := 0
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	ctx := r.Context()

	for {
		select {
		case <-ctx.Done():
			slog.Info("SSE连接已关闭", "task_id", taskID)
			return

		case <-ticker.C:
			snapshot := task.GetSnapshot()

			// 发送进度事件
			progressData, _ := json.Marshal(snapshot)
			fmt.Fprintf(w, "event: progress\ndata: %s\n\n", progressData)

			// 发送新增的日志
			task.mu.RLock()
			logs := task.Logs[lastLogIndex:]
			task.mu.RUnlock()

			for _, log := range logs {
				logData, _ := json.Marshal(map[string]string{
					"level":   "info",
					"message": log,
				})
				fmt.Fprintf(w, "event: log\ndata: %s\n\n", logData)
				lastLogIndex++
			}

			flusher.Flush()

			// 检查任务是否完成
			task.mu.RLock()
			status := task.Status
			task.mu.RUnlock()

			if status == "completed" || status == "failed" || status == "cancelled" {
				// 发送完成事件
				fullSnapshot := task.GetFullSnapshot()
				completeData, _ := json.Marshal(fullSnapshot)
				fmt.Fprintf(w, "event: complete\ndata: %s\n\n", completeData)
				flusher.Flush()

				slog.Info("任务已完成，关闭SSE连接", "task_id", taskID, "status", status)
				return
			}
		}
	}
}

// CancelBatchRegister 取消批量注册任务
func (s *Server) CancelBatchRegister(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("task_id")
	if taskID == "" {
		http.Error(w, "任务ID不能为空", http.StatusBadRequest)
		return
	}

	task, ok := s.batchRegisterTaskManager.GetTask(taskID)
	if !ok {
		http.Error(w, "任务不存在", http.StatusNotFound)
		return
	}

	task.UpdateStatus("cancelled")
	task.AddLog("❌ 任务已被取消")

	slog.Info("批量注册任务已取消", "task_id", taskID)

	response := map[string]string{
		"status": "cancelled",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
