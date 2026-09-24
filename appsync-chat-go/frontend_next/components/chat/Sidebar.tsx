import { MessageSquare, Users, CalendarDays, Files, Search, Settings, LayoutGrid } from "lucide-react";
import { currentUser } from "../../lib/mock-data";

export function Sidebar() {
    return (
    <aside className="sidebar">
        <div className="profile-avatar">{currentUser.avatar}</div>

        <nav>
            <button className="nav-item active"><MessageSquare size={21} /><span>Chat</span></button>
            <button className="nav-item"><Users size={21} /><span>Teams</span></button>
            <button className="nav-item"><CalendarDays size={21} /><span>Calendar</span></button>
            <button className="nav-item"><Files size={21} /><span>Files</span></button>
        </nav>

        <div className="sidebar-bottom">
            <button className="nav-item"><Search size={21} /><span>Search</span></button>
            <button className="nav-item"><Settings size={21} /><span>Settings</span></button>
            <button className="nav-item"><LayoutGrid size={21} /><span>Apps</span></button>
        </div>
    </aside>
    );
}