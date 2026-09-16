import { execFileSync } from "node:child_process";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";

/**
 * A small archive for the browser to read, built the way a real one is.
 *
 * It is generated rather than committed. A database checked into this repository
 * would be one more file somebody could mistake for real data, and the rule here is
 * that no message content of any kind lives in it. Everything below is invented and
 * belongs to nobody.
 *
 * The shape is the Android one because that is the reader with the most to show: an
 * iPhone store keeps no pictures inside itself, only paths to files that are
 * elsewhere, so a recovered photograph can only be demonstrated here.
 */

const schema = `
CREATE TABLE jid (_id INTEGER PRIMARY KEY, user TEXT, server TEXT, raw_string TEXT);
CREATE TABLE chat (_id INTEGER PRIMARY KEY, jid_row_id INTEGER, subject TEXT, sort_timestamp INTEGER);
CREATE TABLE message (
  _id INTEGER PRIMARY KEY, chat_row_id INTEGER, from_me INTEGER, key_id TEXT,
  sender_jid_row_id INTEGER, timestamp INTEGER, message_type INTEGER, text_data TEXT,
  starred INTEGER, origin INTEGER, origination_flags INTEGER);
CREATE TABLE message_media (
  message_row_id INTEGER PRIMARY KEY, mime_type TEXT, media_name TEXT,
  media_caption TEXT, media_duration INTEGER, file_size INTEGER, file_length INTEGER,
  file_hash TEXT, width INTEGER, height INTEGER, file_path TEXT);
CREATE TABLE message_thumbnail (message_row_id INTEGER PRIMARY KEY, thumbnail BLOB);
CREATE TABLE group_participant_user (_id INTEGER PRIMARY KEY, group_jid_row_id INTEGER, user_jid_row_id INTEGER, rank INTEGER);
`;

/** The addresses. The numbers are invented and belong to nobody. */
const ana = "34600111222";
const luis = "34600333444";
const group = "120363001-1500000000";

/** whenMillis is a date as the database stores it. */
function whenMillis(iso: string): number {
  return Date.parse(iso);
}

/**
 * longConversation builds enough messages that the list has to be virtualised.
 *
 * Two hundred is well past what fits on a screen, which is the point: a test that
 * only ever sees ten rows would pass against a viewer that renders them all and
 * falls over on a real conversation.
 */
function longConversation(): string {
  const rows: string[] = [];
  for (let at = 1; at <= 200; at++) {
    const mine = at % 3 === 0 ? 1 : 0;
    const when = whenMillis("2019-06-14T09:00:00Z") + at * 60_000;
    const text = at === 137 ? "the needle in this haystack" : `message number ${String(at)}`;
    rows.push(
      `INSERT INTO message (_id, chat_row_id, from_me, key_id, sender_jid_row_id, timestamp, message_type, text_data)
       VALUES (${String(at)}, 1, ${String(mine)}, 'AAAA${String(at).padStart(12, "0")}', 1, ${String(when)}, 0, '${text}');`,
    );
  }
  return rows.join("\n");
}

/**
 * A picture that outlived its file.
 *
 * One transparent pixel, as a GIF, written as a blob literal. What it depicts does
 * not matter: what is being proved is that a preview stored inside the database
 * reaches the page as an image, which is the most valuable thing this program does.
 */
const onePixelGIF =
  "X'47494638396101000100800000000000FFFFFF21F90401000000002C00000000010001000002024401003B'";

/**
 * A photograph that did not outlive its file, because the file came too.
 *
 * The same one pixel, as a JPEG this time, written to the folder the phone keeps
 * pictures in. The database records where it was and holds none of it, which is what
 * a WhatsApp database actually does; the bytes are on disk beside it, which is what
 * bringing the phone's folder means. Only a browser can prove the picture arrives:
 * jsdom will happily report an <img> whose source fetches a 404.
 */
const onePixelJPEG = Buffer.from(
  "/9j/4AAQSkZJRgABAQEAYABgAAD/2wBDAAgGBgcGBQgHBwcJCQgKDBQNDAsLDBkSEw8UHRofHh0a" +
    "HBwgJC4nICIsIxwcKDcpLDAxNDQ0Hyc5PTgyPC4zNDL/wAALCAABAAEBAREA/8QAFAABAAAAAAAA" +
    "AAAAAAAAAAAACf/EABQQAQAAAAAAAAAAAAAAAAAAAAD/2gAIAQEAAD8AKp//2Q==",
  "base64",
);

/** Where the phone recorded that picture, and where it is written. */
const pictureAt = "Media/WhatsApp Images/IMG-20190615-WA0001.jpg";

const contents = `
INSERT INTO jid (_id, user, server, raw_string) VALUES (1, '${ana}', 's.whatsapp.net', '${ana}@s.whatsapp.net');
INSERT INTO jid (_id, user, server, raw_string) VALUES (2, '${luis}', 's.whatsapp.net', '${luis}@s.whatsapp.net');
INSERT INTO jid (_id, user, server, raw_string) VALUES (3, '${group}', 'g.us', '${group}@g.us');

INSERT INTO chat (_id, jid_row_id, subject) VALUES (1, 1, NULL);
INSERT INTO chat (_id, jid_row_id, subject) VALUES (2, 3, 'Vermut del sabado');

INSERT INTO group_participant_user (_id, group_jid_row_id, user_jid_row_id, rank) VALUES (1, 3, 1, 2);
INSERT INTO group_participant_user (_id, group_jid_row_id, user_jid_row_id, rank) VALUES (2, 3, 2, 0);

${longConversation()}

INSERT INTO message (_id, chat_row_id, from_me, key_id, sender_jid_row_id, timestamp, message_type, text_data)
  VALUES (201, 2, 0, 'BBBB000000000001', 1, ${String(whenMillis("2019-06-15T11:00:00Z"))}, 1, 'look at this');
INSERT INTO message_media (message_row_id, mime_type, media_name, file_size)
  VALUES (201, 'image/gif', 'beach.gif', 43);
INSERT INTO message_thumbnail (message_row_id, thumbnail) VALUES (201, ${onePixelGIF});

INSERT INTO message (_id, chat_row_id, from_me, key_id, sender_jid_row_id, timestamp, message_type, text_data)
  VALUES (202, 2, 0, 'BBBB000000000002', 2, ${String(whenMillis("2019-06-15T12:00:00Z"))}, 0, 'quien se apunta');

INSERT INTO message (_id, chat_row_id, from_me, key_id, sender_jid_row_id, timestamp, message_type, text_data)
  VALUES (203, 2, 0, 'BBBB000000000003', 1, ${String(whenMillis("2019-06-15T13:00:00Z"))}, 1, 'y esta');
INSERT INTO message_media (message_row_id, mime_type, media_name, file_size, file_path)
  VALUES (203, 'image/jpeg', 'playa.jpg', 160, '${pictureAt}');
`;

/**
 * The address book.
 *
 * An Android archive carries no names of its own: WhatsApp reads the phone's
 * contacts at display time and stores only numbers. Without this the conversations
 * are labelled by telephone number, which is correct and is the single most
 * noticeable way an archive disappoints somebody. Passing it here means the path
 * that fixes that is exercised too.
 */
const addressBook = `BEGIN:VCARD
VERSION:3.0
FN:Ana Lopez
TEL:+${ana}
END:VCARD
BEGIN:VCARD
VERSION:3.0
FN:Luis
TEL:+${luis}
END:VCARD
`;

/** A built archive, and the way to remove it again. */
export interface Archive {
  path: string;
  contacts: string;
  remove: () => void;
}

/**
 * build writes the archive and returns where it is.
 *
 * sqlite3 is used rather than a Node library because it is already on every machine
 * this runs on, and adding a native dependency to the frontend to write a fixture
 * would be a poor trade.
 */
export function build(): Archive {
  const directory = mkdtempSync(join(tmpdir(), "amberkeep-e2e-"));
  const path = join(directory, "msgstore.db");
  const contacts = join(directory, "contacts.vcf");

  execFileSync("sqlite3", [path], { input: schema + contents });
  writeFileSync(contacts, addressBook);

  // The phone's folder, beside the database, where the program looks for it without
  // being told.
  const picture = join(directory, ...pictureAt.split("/"));
  mkdirSync(dirname(picture), { recursive: true });
  writeFileSync(picture, onePixelJPEG);

  return {
    path,
    contacts,
    remove: () => {
      rmSync(directory, { recursive: true, force: true });
    },
  };
}
