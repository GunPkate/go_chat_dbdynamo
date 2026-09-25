"use client";

import { ArrowLeft, MoreHorizontal, Phone, Video, Paperclip, Smile, Send, Bot, Hash, UsersRound } from "lucide-react";
import { useChatStore } from "../../chat/store/chat-store";
import { currentUser, users } from "../../lib/mock-data";
import "../css/ChatWindow.css";

export function ChatWindow() {
  const conversations = useChatStore((s) => s.conversations);
  const selectedId = useChatStore((s) => s.selectedId);
  const draft = useChatStore((s) => s.draft);
  const setDraft = useChatStore((s) => s.setDraft);
  const sendMessage = useChatStore((s) => s.sendMessage);
  const setMobileListOpen = useChatStore((s) => s.setMobileListOpen);

  const conversation = conversations.find((c) => c.id === selectedId)!;

  function senderName(senderId: string) {
    if (senderId === currentUser.id) return "You";
    return users.find((u) => u.id === senderId)?.name ?? "AI Assistant";
  }

  return (
    <div className="chat-window">
      <header className="chat-header">
        <button className="mobile-back icon-button" onClick={() => setMobileListOpen(true)}><ArrowLeft size={20} /></button>
        <div className="chat-avatar">
          {conversation.type === "CHANNEL" ? <Hash size={19} /> : conversation.type === "GROUP" ? <UsersRound size={19} /> : conversation.title === "Hotel AI" ? <Bot size={19} /> : conversation.avatar}
        </div>
        <div className="chat-title">
          <strong>{conversation.title}</strong>
          <span>{conversation.subtitle}</span>
        </div>
        <div className="chat-actions">
          <button className="icon-button"><Phone size={19} /></button>
          <button className="icon-button"><Video size={19} /></button>
          <button className="icon-button"><MoreHorizontal size={20} /></button>
        </div>
      </header>

      <div className="chat-context">
        <span className="context-chip">{conversation.type}</span>
        {conversation.reservationId && <span>Reservation {conversation.reservationId.replace("RES#", "")}</span>}
        {conversation.roomId && <span>Room {conversation.roomId.replace("ROOM#", "")}</span>}
        {conversation.channelName && <span>#{conversation.channelName}</span>}
      </div>

      <div className="message-list">
        <div className="day-divider"><span>Today</span></div>
        {conversation.messages.map((message) => {
          const mine = message.senderId === currentUser.id;
          return (
            <div key={message.id} className={`message-row ${mine ? "mine" : ""}`}>
              {!mine && <div className="message-avatar">{message.senderId === "AI#HOTEL" ? "AI" : senderName(message.senderId).slice(0, 1)}</div>}
              <div className="message-body">
                <div className="message-meta">
                  <strong>{senderName(message.senderId)}</strong>
                  <span>{message.createdAt}</span>
                </div>
                <div className="message-bubble">{message.content}</div>
                {message.reactions && (
                  <div className="reactions">
                    {message.reactions.map((r) => <button key={r.emoji}>{r.emoji} {r.count}</button>)}
                  </div>
                )}
              </div>
            </div>
          );
        })}
      </div>

      <div className="composer-wrap">
        <div className="composer-toolbar">
          <button><Paperclip size={18} /></button>
          <button><Smile size={18} /></button>
        </div>
        <div className="composer">
          <textarea
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault();
                sendMessage();
              }
            }}
            placeholder={`Message ${conversation.title}`}
            rows={1}
          />
          <button className="send-button" onClick={sendMessage} disabled={!draft.trim()}><Send size={18} /></button>
        </div>
        <div className="composer-hint">Enter to send · Shift + Enter for new line</div>
      </div>
    </div>
  );
}
