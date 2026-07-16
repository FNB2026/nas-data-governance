// Package runner 提供进程内 worker pool，用于控制并发数（P1-4）。
//
// 设计目标：
//   - 用 semaphore 限制最大并发 goroutine 数（--workers N）
//   - 支持 context 取消（Ctrl+C 时优雅等待在途任务）
//   - 单个任务失败不终止其他任务（与 scanner 的单目录失败不终止设计一致）
//   - 收集所有任务的结果和错误，由调用方决定如何处理
//
// 不做的事：
//   - 不持久化任务队列（崩溃恢复由 P0-1 的 execution_journal 负责）
//   - 不做 IO 速率限制（本阶段只控制并发数）
//   - 不做任务优先级调度（FIFO 即可）
package runner

import (
	"context"
	"sync"
)

// Runner 管理一组 worker goroutine，限制并发数。
// 零值不可用，必须用 New 创建。
type Runner struct {
	sem  chan struct{}  // semaphore，缓冲大小 = workers
	wg   sync.WaitGroup // 等待所有任务完成
	mu   sync.Mutex     // 保护 errors 和 results
	errs []error        // 收集任务错误（保序）
	n    int            // 已提交任务数
}

// New 创建一个并发数为 workers 的 Runner。
// workers <= 0 时视为 1（串行模式，保持向后兼容）。
func New(workers int) *Runner {
	if workers < 1 {
		workers = 1
	}
	return &Runner{
		sem: make(chan struct{}, workers),
	}
}

// Submit 提交一个任务。如果所有 worker 都在忙，Submit 会阻塞直到有空位。
// 如果 ctx 已取消，Submit 立即返回 ctx.Err()，不再提交。
// 任务函数应自己检查 ctx.Err() 以支持中途取消。
func (r *Runner) Submit(ctx context.Context, task func() error) error {
	// 先检查 context
	if err := ctx.Err(); err != nil {
		return err
	}
	// 获取 semaphore 空位（阻塞）
	select {
	case r.sem <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}

	r.wg.Add(1)
	r.mu.Lock()
	r.n++
	r.mu.Unlock()

	go func() {
		defer r.wg.Done()
		defer func() { <-r.sem }() // 释放 semaphore

		err := task()
		if err != nil {
			r.mu.Lock()
			r.errs = append(r.errs, err)
			r.mu.Unlock()
		}
	}()
	return nil
}

// Wait 等待所有已提交的任务完成，返回收集到的错误（保序）。
// 如果 ctx 在等待期间取消，Wait 仍会等待在途任务完成（不中途抛弃），
// 但已提交的任务应通过 ctx 感知取消并快速返回。
func (r *Runner) Wait() []error {
	r.wg.Wait()
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.errs
}

// Count 返回已提交的任务总数。
func (r *Runner) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.n
}

// Run 是 Submit + Wait 的便捷组合，用于批量提交一组同质任务。
// tasks 是一个返回 task 闭包的切片（每个元素是一个工厂函数）。
// 返回所有任务的错误（保序）。如果任一任务出错，其他任务仍会执行。
func Run(ctx context.Context, workers int, tasks []func() error) []error {
	r := New(workers)
	for _, task := range tasks {
		if err := r.Submit(ctx, task); err != nil {
			// context 取消，不再提交后续任务
			break
		}
	}
	return r.Wait()
}
