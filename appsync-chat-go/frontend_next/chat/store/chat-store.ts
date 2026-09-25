
import { create } from "zustand";
import { conversations } from "../../lib/mock-data";
import type { Conversation, Message } from "../../lib/types";

type ChatState = {
    conversations: Conversation[];
    selectedId: string;
    draft: string;
    mobileListOpen: boolean;
    setSelected: (id: string) => void;
    setDraft: (value: string) => void;
    setMobileListOpen: (value: boolean) => void;
    sendMessage: () => void;
};


export const useChatStore = create<ChatState>((set, get) => ({
    conversations: conversations,
    selectedId: conversations[0].id,
    draft: "",
    mobileListOpen: true,

    setSelected: (id: string) => set({ selectedId: id }),
    setDraft: (value: string) => set({ draft: value }),
    setMobileListOpen: (value: boolean) => set({ mobileListOpen: value }),

    sendMessage: () => {
        const { selectedId, draft, conversations } = get();
        const content = draft.trim();
        if (!content) return;

        const message: Message = {
            id: `LOCAL#${Date.now()}`,
            senderId: "USER#201",
            content,
            createdAt: new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
            status: "SENT"
        };

        set((state) => ({
            draft: "",
            conversations: state.conversations.map((c) =>
                c.id === selectedId
                    ? { ...c, messages: [...c.messages, message], unread: 0 }
                    : c
            )
        }));
    }
}));