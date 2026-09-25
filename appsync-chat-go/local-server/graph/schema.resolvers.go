package graph

// Hand-written to gqlgen's standard generated-signature convention (see
// README.md "Wiring the generated resolvers" if `gqlgen generate`
// produces slightly different parameter names on your machine/version —
// the fix is always just forwarding the same arguments, in the same
// order, into the logic.* function below).

import (
	"context"

	"appsync-chat-go/local-server/generated"
	"appsync-chat-go/local-server/logic"
)

// ---------- Query ----------

type queryResolver struct{ *Resolver }

func (r *Resolver) Query() generated.QueryResolver { return &queryResolver{r} }

func (r *queryResolver) GetMessages(ctx context.Context, chatRoomID string, limit *int, since *string) ([]*logic.Message, error) {
	msgs, err := logic.GetMessages(ctx, chatRoomID, limit, since)
	if err != nil {
		return nil, err
	}
	out := make([]*logic.Message, len(msgs))
	for i := range msgs {
		out[i] = &msgs[i]
	}
	return out, nil
}

func (r *queryResolver) ListConversations(ctx context.Context, userID string) ([]*logic.Conversation, error) {
	convos, err := logic.ListConversations(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]*logic.Conversation, len(convos))
	for i := range convos {
		out[i] = &convos[i]
	}
	return out, nil
}

func (r *queryResolver) GetUnreadCount(ctx context.Context, chatRoomID string, userID string) (int, error) {
	return logic.GetUnreadCount(ctx, chatRoomID, userID)
}

// --- Customer side ---

func (r *queryResolver) SearchProducts(ctx context.Context, query string) (*logic.AssistantReply, error) {
	return logic.SearchProducts(ctx, query)
}

func (r *queryResolver) RecommendedProducts(ctx context.Context, freeTextContext *string) (*logic.AssistantReply, error) {
	return logic.RecommendProducts(ctx, freeTextContext)
}

func (r *queryResolver) SubstituteProducts(ctx context.Context, productID string) (*logic.AssistantReply, error) {
	return logic.SubstituteProducts(ctx, productID)
}

// --- Employee side ---

func (r *queryResolver) StockInsights(ctx context.Context, question string) (*logic.AssistantReply, error) {
	return logic.StockInsights(ctx, question)
}

func (r *queryResolver) StaffAssistant(ctx context.Context, question string) (*logic.AssistantReply, error) {
	return logic.StaffAssistant(ctx, question)
}

// ---------- Mutation ----------

type mutationResolver struct{ *Resolver }

func (r *Resolver) Mutation() generated.MutationResolver { return &mutationResolver{r} }

func (r *mutationResolver) SendMessage(ctx context.Context, chatRoomID string, sender string, content string, clientToken string, attachmentKey *string, participants []string) (*logic.Message, error) {
	return logic.SendMessage(ctx, chatRoomID, sender, content, clientToken, attachmentKey, participants)
}

func (r *mutationResolver) EditMessage(ctx context.Context, chatRoomID string, createdAt string, id string, content string) (*logic.Message, error) {
	return logic.EditMessage(ctx, chatRoomID, createdAt, id, content)
}

func (r *mutationResolver) DeleteMessage(ctx context.Context, chatRoomID string, createdAt string, id string) (*logic.Message, error) {
	return logic.DeleteMessage(ctx, chatRoomID, createdAt, id)
}

func (r *mutationResolver) MarkAsRead(ctx context.Context, chatRoomID string, createdAt string, id string, reader string) (*logic.Message, error) {
	return logic.MarkAsRead(ctx, chatRoomID, createdAt, id, reader)
}

func (r *mutationResolver) MarkAsDelivered(ctx context.Context, chatRoomID string, createdAt string, id string) (*logic.Message, error) {
	return logic.MarkAsDelivered(ctx, chatRoomID, createdAt, id)
}

func (r *mutationResolver) SendTypingEvent(ctx context.Context, chatRoomID string, sender string, isTyping bool) (*logic.TypingEvent, error) {
	return logic.SendTypingEvent(chatRoomID, sender, isTyping)
}

func (r *mutationResolver) SetPresence(ctx context.Context, userID string, online bool) (*logic.PresenceEvent, error) {
	return logic.SetPresence(userID, online)
}

// ---------- Subscription ----------

type subscriptionResolver struct{ *Resolver }

func (r *Resolver) Subscription() generated.SubscriptionResolver { return &subscriptionResolver{r} }

func (r *subscriptionResolver) OnMessageSent(ctx context.Context, chatRoomID string) (<-chan *logic.Message, error) {
	return bridgeMessages(ctx, logic.MessageSentBroker, chatRoomID), nil
}

func (r *subscriptionResolver) OnMessageEdited(ctx context.Context, chatRoomID string) (<-chan *logic.Message, error) {
	return bridgeMessages(ctx, logic.MessageEditedBroker, chatRoomID), nil
}

func (r *subscriptionResolver) OnMessageDeleted(ctx context.Context, chatRoomID string) (<-chan *logic.Message, error) {
	return bridgeMessages(ctx, logic.MessageDeletedBroker, chatRoomID), nil
}

func (r *subscriptionResolver) OnReadReceipt(ctx context.Context, chatRoomID string) (<-chan *logic.Message, error) {
	return bridgeMessages(ctx, logic.ReadReceiptBroker, chatRoomID), nil
}

func (r *subscriptionResolver) OnDeliveredReceipt(ctx context.Context, chatRoomID string) (<-chan *logic.Message, error) {
	return bridgeMessages(ctx, logic.DeliveredReceiptBroker, chatRoomID), nil
}

func (r *subscriptionResolver) OnTyping(ctx context.Context, chatRoomID string) (<-chan *logic.TypingEvent, error) {
	ch, cancel := logic.TypingBroker.Subscribe(chatRoomID)
	out := make(chan *logic.TypingEvent, 1)
	go func() {
		defer cancel()
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-ch:
				if !ok {
					return
				}
				e := ev
				select {
				case out <- &e:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}

func (r *subscriptionResolver) OnPresenceChanged(ctx context.Context, userID string) (<-chan *logic.PresenceEvent, error) {
	ch, cancel := logic.PresenceBroker.Subscribe(userID)
	out := make(chan *logic.PresenceEvent, 1)
	go func() {
		defer cancel()
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-ch:
				if !ok {
					return
				}
				e := ev
				select {
				case out <- &e:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}

// bridgeMessages adapts a Broker[logic.Message] subscription into the
// <-chan *logic.Message shape a gqlgen subscription resolver returns,
// and unsubscribes/cleans up once the client disconnects (ctx cancelled).
func bridgeMessages(ctx context.Context, broker *logic.Broker[logic.Message], topic string) <-chan *logic.Message {
	ch, cancel := broker.Subscribe(topic)
	out := make(chan *logic.Message, 1)
	go func() {
		defer cancel()
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				m := msg
				select {
				case out <- &m:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out
}
