package channel

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

type inboundTask struct {
	ctx         context.Context
	cfg         ChannelConfig
	msg         InboundMessage
	enqueueTime time.Time // 消息入队时间，用于计算队列等待时间
}

// HandleInbound enqueues an inbound message for asynchronous processing by the worker pool.
func (m *Manager) HandleInbound(ctx context.Context, cfg ChannelConfig, msg InboundMessage) error {
	if m.processor == nil {
		return fmt.Errorf("inbound processor not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	m.startInboundWorkers(ctx)
	if m.inboundCtx != nil && m.inboundCtx.Err() != nil {
		return fmt.Errorf("inbound dispatcher stopped")
	}
	task := inboundTask{
		ctx:         context.WithoutCancel(ctx),
		cfg:         cfg,
		msg:         msg,
		enqueueTime: time.Now(),
	}
	queueLen := len(m.inboundQueue)
	select {
	case m.inboundQueue <- task:
		if m.logger != nil {
			m.logger.Info("[PERF] message enqueued",
				slog.String("channel", msg.Channel.String()),
				slog.String("bot_id", cfg.BotID),
				slog.Int("queue_len", queueLen),
				slog.Int("queue_capacity", cap(m.inboundQueue)))
		}
		return nil
	default:
		if m.logger != nil {
			m.logger.Error("[PERF] inbound queue full",
				slog.String("channel", msg.Channel.String()),
				slog.String("bot_id", cfg.BotID),
				slog.Int("queue_capacity", cap(m.inboundQueue)))
		}
		return fmt.Errorf("inbound queue full")
	}
}

func (m *Manager) handleInbound(ctx context.Context, cfg ChannelConfig, msg InboundMessage) error {
	if m.processor == nil {
		return fmt.Errorf("inbound processor not configured")
	}
	sender := m.newReplySender(cfg, msg.Channel)
	if err := m.processor.HandleInbound(ctx, cfg, msg, sender); err != nil {
		if m.logger != nil {
			m.logger.Error("inbound processing failed", slog.String("channel", msg.Channel.String()), slog.Any("error", err))
		}
		return err
	}
	return nil
}

func (m *Manager) startInboundWorkers(ctx context.Context) {
	m.inboundOnce.Do(func() {
		workerCtx := ctx
		if workerCtx == nil {
			workerCtx = context.Background()
		}
		m.inboundCtx, m.inboundCancel = context.WithCancel(workerCtx)
		for i := 0; i < m.inboundWorkers; i++ {
			go m.runInboundWorker(m.inboundCtx)
		}
	})
}

func (m *Manager) runInboundWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case task := <-m.inboundQueue:
			// 计算队列等待时间
			queueWaitTime := time.Since(task.enqueueTime)
			if m.logger != nil && queueWaitTime > 100*time.Millisecond {
				m.logger.Info("[PERF] message dequeued",
					slog.String("channel", task.msg.Channel.String()),
					slog.String("bot_id", task.cfg.BotID),
					slog.Duration("queue_wait_time", queueWaitTime),
					slog.Int("queue_len", len(m.inboundQueue)))
			}
			if err := m.handleInbound(task.ctx, task.cfg, task.msg); err != nil {
				if m.logger != nil {
					m.logger.Error("inbound processing failed", slog.String("channel", task.msg.Channel.String()), slog.Any("error", err))
				}
			}
		}
	}
}
