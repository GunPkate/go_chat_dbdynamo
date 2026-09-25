import { ChatWindow } from "./ChatWindow";
import { ConversationList } from "./ConversationList";
import { Sidebar } from "./Sidebar";
import "../css/ChatApp.css";

export function ChatApp() {
    return (
    <main className="app-shell">
      <Sidebar />
      <section className={`conversation-pane`}>
        <ConversationList />
      </section>
      <section className={`chat-pane`}>
        <ChatWindow />
      </section>
    </main>
    );
}