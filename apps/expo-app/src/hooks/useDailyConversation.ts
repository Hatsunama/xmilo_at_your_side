import { useCallback, useEffect, useRef, useState } from "react";
import type { LairDailyChatMessage, PendingChatMessage } from "../lib/lairDailyChat";
import {
  appendLairChatMessage,
  chatProjectionFromRuntimeEvent,
  listTodayLairChatMessages,
  localChatDate,
  removeLairChatMessagesForTaskSource,
  resetLairChatAfterNightlyArchive,
  updateLairChatMessageTaskIdentity,
} from "../lib/lairDailyChat";
import { taskIdFromEvent } from "../lib/runtimeEvents";
import type { EventEnvelope } from "../types/contracts";

export function useDailyConversation(events: EventEnvelope[], currentTaskId?: string) {
  const [messages, setMessages] = useState<LairDailyChatMessage[]>([]);
  const currentChatDateRef = useRef(localChatDate());
  const processedEventIdsRef = useRef(new Set<string>());
  const reportReadyTaskIdsRef = useRef(new Set<string>());
  const chatNearBottomRef = useRef(true);
  const forceNextChatScrollRef = useRef(false);

  const loadDailyConversation = useCallback(async () => {
    const today = localChatDate();
    currentChatDateRef.current = today;
    const nextMessages = await listTodayLairChatMessages(today);
    setMessages(nextMessages);
    reportReadyTaskIdsRef.current = new Set(
      nextMessages
        .filter((message) => message.source_event_type === "report.ready" && message.task_id)
        .map((message) => String(message.task_id))
    );
  }, []);

  const appendProjectedMessage = useCallback(async (message: PendingChatMessage, forceScroll = false) => {
    const saved = await appendLairChatMessage(message, currentChatDateRef.current);
    if (!saved) return null;
    setMessages((prev) => (prev.some((item) => item.id === saved.id) ? prev : [...prev, saved]));
    if (forceScroll || chatNearBottomRef.current) {
      forceNextChatScrollRef.current = true;
    }
    return saved;
  }, []);

  const appendUserPrompt = useCallback(
    async (text: string) => {
      const userMessageId = `user:${new Date().toISOString()}:${Math.random().toString(36).slice(2)}`;
      await appendProjectedMessage(
        {
          id: userMessageId,
          role: "user",
          text,
          source_event_type: "ui.prompt_submitted",
        },
        true
      );
      return userMessageId;
    },
    [appendProjectedMessage]
  );

  const updateMessageTaskIdentity = useCallback(async (id: string, taskId?: string | null, attemptId?: string | null) => {
    await updateLairChatMessageTaskIdentity(id, taskId, attemptId);
    setMessages((prev) =>
      prev.map((message) =>
        message.id === id
          ? { ...message, task_id: taskId || message.task_id, attempt_id: attemptId || message.attempt_id }
          : message
      )
    );
  }, []);

  useEffect(() => {
    let disposed = false;
    async function load() {
      try {
        await loadDailyConversation();
      } catch {
        if (!disposed) setMessages([]);
      }
    }
    void load();
    return () => {
      disposed = true;
    };
  }, [loadDailyConversation]);

  useEffect(() => {
    let disposed = false;
    async function projectEvents() {
      for (const event of events) {
        const taskId = taskIdFromEvent(event);
        const eventKey = `${event.type}:${event.timestamp}:${taskId}:${String(event.payload?.attempt_id ?? "")}`;
        if (processedEventIdsRef.current.has(eventKey)) continue;
        processedEventIdsRef.current.add(eventKey);

        if (event.type === "maintenance.nightly_completed") {
          await resetLairChatAfterNightlyArchive(event.payload?.archive_date);
          if (disposed) return;
          await loadDailyConversation();
          continue;
        }

        if (event.type === "report.ready" && taskId) {
          reportReadyTaskIdsRef.current.add(taskId);
          await removeLairChatMessagesForTaskSource(taskId, "task.completed");
          if (!disposed) {
            setMessages((prev) => prev.filter((message) => !(message.task_id === taskId && message.source_event_type === "task.completed")));
          }
        }

        const projection = chatProjectionFromRuntimeEvent(event, {
          reportReadyTaskIds: reportReadyTaskIdsRef.current,
          currentTaskId,
        });
        if (projection) {
          await appendProjectedMessage(projection);
        }
      }
    }
    void projectEvents();
    return () => {
      disposed = true;
    };
  }, [appendProjectedMessage, currentTaskId, events, loadDailyConversation]);

  return {
    messages,
    appendUserPrompt,
    updateMessageTaskIdentity,
    loadDailyConversation,
    chatNearBottomRef,
    forceNextChatScrollRef,
  };
}
