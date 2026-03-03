package repository

import (
	"context"
	"strings"
	"time"

	"github.com/sjzar/chatlog/internal/model"
	"github.com/sjzar/chatlog/pkg/util"

	"github.com/rs/zerolog/log"
)

// GetMessages implements Repository interface's GetMessages method
func (r *Repository) GetMessages(ctx context.Context, startTime, endTime time.Time, talker string, sender string, keyword string, limit, offset int) ([]*model.Message, error) {

	talker, sender = r.parseTalkerAndSender(ctx, talker, sender)
	messages, err := r.ds.GetMessages(ctx, startTime, endTime, talker, sender, keyword, limit, offset)
	if err != nil {
		return nil, err
	}

	// Enrich message info
	if err := r.EnrichMessages(ctx, messages); err != nil {
		log.Debug().Msgf("EnrichMessages failed: %v", err)
	}

	return messages, nil
}

// EnrichMessages enriches messages with extra info
func (r *Repository) EnrichMessages(ctx context.Context, messages []*model.Message) error {
	for _, msg := range messages {
		r.enrichMessage(msg)
	}
	return nil
}

// enrichMessage enriches a single message with extra info
func (r *Repository) enrichMessage(msg *model.Message) {
	// Handle chat room messages
	if msg.IsChatRoom {
		// Enrich chat room name
		if chatRoom, ok := r.chatRoomCache[msg.Talker]; ok {
			msg.TalkerName = chatRoom.DisplayName()

			// Enrich sender's display name in group
			if displayName, ok := chatRoom.User2DisplayName[msg.Sender]; ok {
				msg.SenderName = displayName
			}
		}
	}

	// If not self-sent and no display name yet, try to enrich sender info
	if msg.SenderName == "" && !msg.IsSelf {
		contact := r.getFullContact(msg.Sender)
		if contact != nil {
			msg.SenderName = contact.DisplayName()
		}
	}
}

func (r *Repository) parseTalkerAndSender(ctx context.Context, talker, sender string) (string, string) {
	displayName2User := make(map[string]string)
	users := make(map[string]bool)

	talkers := util.Str2List(talker, ",")
	if len(talkers) > 0 {
		for i := 0; i < len(talkers); i++ {
			if contact, _ := r.GetContact(ctx, talkers[i]); contact != nil {
				talkers[i] = contact.UserName
			} else if chatRoom, _ := r.GetChatRoom(ctx, talker); chatRoom != nil {
				talkers[i] = chatRoom.Name
			}
		}
		// Get chat room user list
		for i := 0; i < len(talkers); i++ {
			if chatRoom, _ := r.GetChatRoom(ctx, talkers[i]); chatRoom != nil {
				for user, displayName := range chatRoom.User2DisplayName {
					displayName2User[displayName] = user
				}
				for _, user := range chatRoom.Users {
					users[user.UserName] = true
				}
			}
		}
		talker = strings.Join(talkers, ",")
	}

	senders := util.Str2List(sender, ",")
	if len(senders) > 0 {
		for i := 0; i < len(senders); i++ {
			if user, ok := displayName2User[senders[i]]; ok {
				senders[i] = user
			} else {
				// FIXME Many chat room users have duplicate names, cannot get ID via GetContact directly, optimize later
				for user := range users {
					if contact := r.getFullContact(user); contact != nil {
						if contact.DisplayName() == senders[i] {
							senders[i] = user
							break
						}
					}
				}
			}
		}
		sender = strings.Join(senders, ",")
	}

	return talker, sender
}
