"use client";
import { Search, Plus, Pin, Hash, UsersRound, Bot } from "lucide-react";
import "../css/ConversationList.css";
import { useChatStore } from "../../chat/store/chat-store";

export function ConversationList() {
  const conversations = useChatStore((s) => s.conversations);
  const selectedId = useChatStore((s) => s.selectedId);
  const setSelected = useChatStore((s) => s.setSelected);

  return (
    <div className="conversation-list">
      <header className="list-header">
        <div>
          <h1>Chat</h1>
          <span className="muted">Hotel & Retail</span>
        </div>
        <button className="icon-button" aria-label="New chat"><Plus size={20} /></button>
      </header>

      <div className="search-box">
        <Search size={17} />
        <input placeholder="Search messages, people..." />
      </div>

      <div className="list-section">
        <div className="section-title">Recent</div>
        {conversations.map((conversation) => {
          const Icon = conversation.type === "CHANNEL" ? Hash : conversation.title === "Hotel AI" ? Bot : conversation.type === "GROUP" ? UsersRound : null;
          return (
            <button
              key={conversation.id}
              className={`conversation-item ${selectedId === conversation.id ? "selected" : ""}`}
              onClick={() => setSelected(conversation.id)}
            >
              <div className="avatar-wrap">
                <div className="conversation-avatar">{Icon ? <Icon size={18} /> : conversation.avatar}</div>
                {conversation.online && <span className="online-dot" />}
              </div>
              <div className="conversation-main">
                <div className="conversation-top">
                  <strong>{conversation.title}</strong>
                  <span className="time">{conversation.messages.at(-1)?.createdAt}</span>
                </div>
                <div className="conversation-bottom">
                  <span>{conversation.messages.at(-1)?.content}</span>
                  {conversation.unread > 0 && <b className="unread">{conversation.unread}</b>}
                </div>
              </div>
            </button>
          );
        })}
      </div>

      <div className="list-footer">
        <Pin size={15} /> Pinned conversations
      </div>
    </div>
  );
}