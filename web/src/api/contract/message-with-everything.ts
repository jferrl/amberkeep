// Recorded by internal/api/contract_test.go. Do not edit; re-record it:
//
//   go test ./internal/api -run TestTheRecordedRepliesStillMatch -update
//
// This is what the server actually sent. `as const` keeps every string literal,
// so the unions the page declares are checked rather than widened to `string`.

/**
 * The same reply, carrying every optional field a message can carry.
 * 
 * No real message looks like this. Each field has to appear in some recording or
 * nothing checks its shape, and one message costs one recording instead of twenty.
 */
export default {
  "chat": {
    "id": 4,
    "address": "120363999@g.us",
    "kind": "group",
    "name": "Everything at once",
    "participants": [
      {
        "address": "34600111222@s.whatsapp.net",
        "name": "Ana Lopez",
        "admin": true
      }
    ],
    "description": "A conversation that exists to be written down",
    "created_at": "2019-06-13T09:00:00Z",
    "last_message_at": "2019-06-14T11:00:00Z",
    "message_count": 1
  },
  "messages": [
    {
      "id": 7,
      "key": "3EB0C431C26A1D4F",
      "sent_at": "2019-06-14T11:00:00Z",
      "from_me": false,
      "sender": "34600111222@s.whatsapp.net",
      "sender_name": "Ana Lopez",
      "kind": "image",
      "text": "look at this",
      "rendered": "This message was deleted by Luis",
      "attachment": {
        "media_type": "image/jpeg",
        "file_name": "beach.jpg",
        "size": 184320,
        "width": 1600,
        "height": 1200,
        "caption": "look at this",
        "preview_base64": "/9j/4AAQSkZJRv/Z"
      },
      "reply_to": {
        "sender": "34600333444@s.whatsapp.net",
        "sender_name": "Luis",
        "kind": "text",
        "text": "where are you?"
      },
      "reactions": [
        {
          "emoji": "❤️",
          "sender": "34600333444@s.whatsapp.net",
          "sender_name": "Luis",
          "at": "2019-06-14T11:01:00Z"
        }
      ],
      "mentions": [
        "34600333444@s.whatsapp.net"
      ],
      "place": {
        "latitude": 41.3874,
        "longitude": 2.1686,
        "name": "Bogatell",
        "address": "Passeig Maritim",
        "url": "https://example.invalid/map",
        "live": true
      },
      "poll": {
        "question": "Vermut on Saturday?",
        "options": [
          {
            "name": "Yes",
            "votes": 2
          },
          {
            "name": "No",
            "votes": 0
          }
        ],
        "closed": true
      },
      "call": {
        "video": true,
        "group": true,
        "outcome": "missed",
        "duration": "1m 32s"
      },
      "link": {
        "url": "https://example.invalid/a",
        "title": "A page",
        "description": "That may no longer exist"
      },
      "contact_cards": [
        {
          "name": "Luis",
          "vcard": "BEGIN:VCARD\nVERSION:3.0\nFN:Luis\nEND:VCARD\n",
          "addresses": [
            "34600333444@s.whatsapp.net"
          ]
        }
      ],
      "group_invite": {
        "group_name": "Vermut",
        "address": "120363001@g.us",
        "expires_at": "2019-06-17T09:00:00Z"
      },
      "notice": {
        "action": 12,
        "text": "Ana Lopez changed the subject",
        "actor": "34600111222@s.whatsapp.net",
        "targets": [
          "34600333444@s.whatsapp.net"
        ],
        "old_value": "Vermut",
        "new_value": "Vermut del sabado",
        "identified": false
      },
      "deleted": {
        "at": "2019-06-14T12:00:00Z",
        "by": "34600333444@s.whatsapp.net",
        "by_admin": true
      },
      "album_size": 3,
      "expires_after": "168h 0m 0s",
      "starred": true,
      "forwarded": true,
      "forward_score": 5,
      "edited_at": "2019-06-14T11:05:00Z",
      "source_type": 1
    }
  ]
} as const;
