//! Long-poll loop that pulls tasks from `novanas-meta` and dispatches
//! each to the matching handler.
//!
//! The runner loops:
//!   1. `MetaService::PollTasks(node_id, max_tasks, deadline_ms)`.
//!   2. For each `Task`, run [`task_handlers::handle_task`] in parallel
//!      (bounded by `concurrency`).
//!   3. `MetaService::AckTask(id, success, error_message)` for each.
//!
//! Failures bubble up as a non-success ack but never abort the loop; the
//! meta side decides whether to requeue. The runner stops cleanly when
//! the cancellation token is fired by the binary's signal handler.

use std::sync::Arc;
use std::time::Duration;

use tokio::sync::{Notify, Semaphore};

use crate::error::Result;
use crate::meta_client::MetaClient;
use crate::task_handlers::{handle_task, HandlerContext};

/// Configuration for the runner.
#[derive(Debug, Clone)]
pub struct TaskRunnerConfig {
    /// Identifier the daemon registers itself with on the meta side.
    /// Wrapper-style PollTasks no longer carries a node_id, but the
    /// dataplane keeps the field for use as a `Heartbeat.client_id` and
    /// for logging.
    pub node_id: String,
    /// Max tasks per poll batch.
    pub max_tasks: u32,
    /// Wrapper-style PollTasks does not long-poll; the runner sleeps
    /// `idle_backoff` between empty batches to avoid busy-looping.
    pub idle_backoff: Duration,
    /// Max concurrent task handlers.
    pub concurrency: usize,
    /// Backoff on transport errors.
    pub error_backoff: Duration,
}

impl Default for TaskRunnerConfig {
    fn default() -> Self {
        Self {
            node_id: hostname_or_unknown(),
            max_tasks: 16,
            idle_backoff: Duration::from_millis(500),
            concurrency: 8,
            error_backoff: Duration::from_secs(2),
        }
    }
}

fn hostname_or_unknown() -> String {
    std::env::var("HOSTNAME").unwrap_or_else(|_| "novanas-data".to_string())
}

/// Cancellation handle for [`TaskRunner::run`].
#[derive(Default, Clone)]
pub struct ShutdownToken {
    notify: Arc<Notify>,
    cancelled: Arc<std::sync::atomic::AtomicBool>,
}

impl ShutdownToken {
    pub fn new() -> Self {
        Self::default()
    }

    /// Trip the token; any active runner stops after the current batch.
    pub fn cancel(&self) {
        self.cancelled
            .store(true, std::sync::atomic::Ordering::SeqCst);
        self.notify.notify_waiters();
    }

    pub fn is_cancelled(&self) -> bool {
        self.cancelled.load(std::sync::atomic::Ordering::SeqCst)
    }

    pub async fn cancelled(&self) {
        if self.is_cancelled() {
            return;
        }
        self.notify.notified().await;
    }
}

/// The runner. Hold an instance to keep the loop alive.
pub struct TaskRunner {
    cfg: TaskRunnerConfig,
    handler_ctx: Arc<HandlerContext>,
}

impl TaskRunner {
    pub fn new(cfg: TaskRunnerConfig, handler_ctx: Arc<HandlerContext>) -> Self {
        Self { cfg, handler_ctx }
    }

    /// Run until `shutdown` is fired.
    pub async fn run(&self, mut client: MetaClient, shutdown: ShutdownToken) -> Result<()> {
        let semaphore = Arc::new(Semaphore::new(self.cfg.concurrency.max(1)));
        loop {
            if shutdown.is_cancelled() {
                log::info!("task_runner: shutdown requested, exiting loop");
                return Ok(());
            }

            tokio::select! {
                _ = shutdown.cancelled() => {
                    log::info!("task_runner: cancellation token fired");
                    return Ok(());
                }
                poll = client.poll_tasks(self.cfg.max_tasks) => {
                    match poll {
                        Ok(resp) => {
                            let was_empty = resp.tasks.is_empty();
                            self.dispatch_batch(&mut client, &semaphore, resp.tasks).await;
                            if was_empty {
                                tokio::time::sleep(self.cfg.idle_backoff).await;
                            }
                        }
                        Err(e) => {
                            log::warn!("task_runner: poll_tasks failed: {e} (backing off)");
                            tokio::time::sleep(self.cfg.error_backoff).await;
                        }
                    }
                }
            }
        }
    }

    async fn dispatch_batch(
        &self,
        client: &mut MetaClient,
        semaphore: &Arc<Semaphore>,
        tasks: Vec<crate::transport::meta_proto::Task>,
    ) {
        if tasks.is_empty() {
            return;
        }
        let mut join_set = tokio::task::JoinSet::new();
        for task in tasks {
            let sem = semaphore.clone();
            let ctx = self.handler_ctx.clone();
            join_set.spawn(async move {
                let _permit = sem.acquire().await.expect("semaphore closed");
                let result = handle_task(&ctx, &task).await;
                (task.id, result)
            });
        }
        while let Some(joined) = join_set.join_next().await {
            match joined {
                Ok((task_id, Ok(()))) => {
                    if let Err(e) = client.ack_task(&task_id, true, "").await {
                        log::warn!("task_runner: ack(success) for {task_id} failed: {e}");
                    }
                }
                Ok((task_id, Err(e))) => {
                    let msg = format!("{e}");
                    log::warn!("task_runner: task {task_id} failed: {msg}");
                    if let Err(ack_err) = client.ack_task(&task_id, false, &msg).await {
                        log::warn!("task_runner: ack(failure) for {task_id} failed: {ack_err}");
                    }
                }
                Err(join_err) => {
                    log::error!("task_runner: handler task panicked: {join_err}");
                }
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn shutdown_token_signals_cancelled() {
        let token = ShutdownToken::new();
        assert!(!token.is_cancelled());
        let watcher = {
            let t = token.clone();
            tokio::spawn(async move {
                t.cancelled().await;
                t.is_cancelled()
            })
        };
        token.cancel();
        let was_cancelled = watcher.await.unwrap();
        assert!(was_cancelled);
    }

    #[test]
    fn config_defaults_are_sane() {
        let c = TaskRunnerConfig::default();
        assert!(c.max_tasks > 0);
        assert!(!c.idle_backoff.is_zero());
        assert!(c.concurrency > 0);
    }
}
