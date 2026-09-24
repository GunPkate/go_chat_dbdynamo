import type { Conversation, User } from "./types";

export const currentUser: User = {
  id: "USER#201",
  name: "Hotel Staff",
  role: "EMPLOYEE",
  avatar: "HS",
  online: true
};

export const users: User[] = [
  { id: "USER#102", name: "Gun", role: "CUSTOMER", avatar: "G", online: true },
  { id: "USER#103", name: "John", role: "CUSTOMER", avatar: "J", online: false },
  { id: "USER#104", name: "Sarah", role: "EMPLOYEE", avatar: "S", online: true },
  { id: "AI#HOTEL", name: "Hotel AI", role: "AI", avatar: "AI", online: true }
];

export const conversations: Conversation[] = [
  {
    id: "CONV#01J8GUN",
    displayKey: "GUN-20260924-134700",
    type: "DIRECT",
    title: "Gun",
    subtitle: "Room 501 · Reservation R002",
    avatar: "G",
    unread: 2,
    online: true,
    customerId: "CUST#102",
    reservationId: "RES#R002",
    roomId: "ROOM#501",
    messages: [
      { id: "MSG#001", senderId: "USER#102", content: "Hi, can I get extra towels?", createdAt: "13:50" },
      { id: "MSG#002", senderId: "USER#201", content: "Sure. I will send them to room 501.", createdAt: "13:51" },
      { id: "MSG#003", senderId: "USER#102", content: "Thank you!", createdAt: "13:52" }
    ]
  },
  {
    id: "CONV#TEAM",
    displayKey: "WAREHOUSE-20260924",
    type: "CHANNEL",
    title: "Warehouse",
    subtitle: "Retail Team · Stock",
    avatar: "#",
    unread: 5,
    teamName: "Retail Team",
    channelName: "Stock",
    messages: [
      { id: "MSG#101", senderId: "USER#104", content: "Milk stock is below reorder level.", createdAt: "12:30" },
      { id: "MSG#102", senderId: "USER#201", content: "I'll check today's delivery.", createdAt: "12:32" }
    ]
  },
  {
    id: "CONV#GROUP",
    displayKey: "STOREOPS-20260924",
    type: "GROUP",
    title: "Store Operations",
    subtitle: "4 members",
    avatar: "SO",
    unread: 0,
    messages: [
      { id: "MSG#201", senderId: "USER#104", content: "POS #3 is ready.", createdAt: "11:05" },
      { id: "MSG#202", senderId: "USER#201", content: "Great, thanks.", createdAt: "11:07" }
    ]
  },
  {
    id: "CONV#AI",
    displayKey: "HOTELAI-20260924",
    type: "DIRECT",
    title: "Hotel AI",
    subtitle: "AI Assistant",
    avatar: "AI",
    unread: 0,
    online: true,
    messages: [
      { id: "MSG#301", senderId: "USER#201", content: "Which guest requests are still open?", createdAt: "10:10" },
      { id: "MSG#302", senderId: "AI#HOTEL", content: "There are 3 open requests: extra towels, airport transfer, and late checkout.", createdAt: "10:10" }
    ]
  }
];
