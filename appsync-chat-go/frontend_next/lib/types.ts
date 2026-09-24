export type ConversationType = "DIRECT" | "GROUP" | "CHANNEL";

export type User = {
  id: string;
  name: string;
  role: "CUSTOMER" | "EMPLOYEE" | "AI";
  avatar: string;
  online?: boolean;
};

export type Message = {
  id: string;
  senderId: string;
  content: string;
  createdAt: string;
  status?: "SENT" | "SENDING" | "FAILED";
  replyToMessageId?: string;
  reactions?: { emoji: string; count: number; reacted: boolean }[];
};

export type Conversation = {
  id: string;
  displayKey: string;
  type: ConversationType;
  title: string;
  subtitle?: string;
  avatar: string;
  unread: number;
  online?: boolean;
  customerId?: string;
  reservationId?: string;
  roomId?: string;
  teamName?: string;
  channelName?: string;
  messages: Message[];
};
