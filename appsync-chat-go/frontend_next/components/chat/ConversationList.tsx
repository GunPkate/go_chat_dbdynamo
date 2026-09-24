import { Search, Plus, Pin, Hash, UsersRound, Bot } from "lucide-react";

export function ConversationList() {
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

      </div>

      <div className="list-footer">
        <Pin size={15} /> Pinned conversations
      </div>
    </div>
  );
}