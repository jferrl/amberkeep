import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { Message as ArchivedMessage } from "@/api/types";
import { Message } from "@/components/Message";
import { render } from "@/test/render";

/** said builds the smallest message the component will accept. */
function said(fields: Partial<ArchivedMessage> = {}): ArchivedMessage {
  return {
    id: 1,
    sent_at: "2019-06-14T09:12:00Z",
    from_me: false,
    kind: "text",
    rendered: "",
    source_type: 0,
    sender_name: "Ana Lopez",
    ...fields,
  };
}

/**
 * withoutSender is a message the archive recorded no sender for.
 *
 * The field is removed rather than set to undefined, because that is what the
 * server does: a field it has nothing for is absent from the JSON entirely, and the
 * types say so.
 */
function withoutSender(fields: Partial<ArchivedMessage> = {}): ArchivedMessage {
  const message = said(fields);
  delete message.sender_name;
  return message;
}

/**
 * A message is words somebody else wrote, and this page renders it in a browser.
 * Nothing it contains may ever become markup. This is the test that says so.
 *
 * The linter forbids the property that would allow it, and these cases check the
 * outcome rather than the rule: whatever somebody sent arrives on screen as the
 * characters they typed.
 */
describe("a message never becomes markup", () => {
  const attempts = [
    { name: "a script tag", text: "<script>alert(1)</script>" },
    { name: "an image with a handler", text: `<img src=x onerror="alert(1)">` },
    {
      name: "an attempt to close the bubble",
      text: "</div></article><script>x</script>",
    },
    { name: "an entity", text: "&lt;not decoded&gt;" },
    { name: "an ampersand, which is not an attack", text: "tea & biscuits" },
  ] as const;

  it.each(attempts)("$name", ({ text }) => {
    const { container } = render(
      <Message message={said({ text })} language="en" />,
    );

    // The characters are on screen exactly as they were sent.
    expect(screen.getByText(text)).toBeInTheDocument();
    // And nothing was created from them.
    expect(container.querySelector("script")).toBeNull();
    expect(container.querySelector("img")).toBeNull();
  });

  it("shows a quoted message as text as well", () => {
    const text = "<b>quoted</b>";
    render(
      <Message
        message={said({
          text: "yes",
          reply_to: { kind: "text", text, sender_name: "Luis" },
        })}
        language="en"
      />,
    );
    expect(screen.getByText(text)).toBeInTheDocument();
  });
});

describe("what a message shows", () => {
  it("names whoever wrote it", () => {
    render(<Message message={said({ text: "hola" })} language="en" />);
    expect(screen.getByText("Ana Lopez")).toBeInTheDocument();
  });

  it("calls the archive's owner You", () => {
    render(
      <Message
        message={withoutSender({ from_me: true, text: "hola" })}
        language="en"
      />,
    );
    expect(screen.getByText("You")).toBeInTheDocument();
  });

  it("says so in Spanish when the page is Spanish", () => {
    render(
      <Message
        message={withoutSender({ from_me: true, text: "hola" })}
        language="es"
      />,
      "es",
    );
    expect(screen.getByText("Tú")).toBeInTheDocument();
  });

  /**
   * The server sends both the words and a line describing what the message was when
   * it was not words. Showing both for an ordinary message would repeat it.
   */
  it("describes a message that was not words", () => {
    render(
      <Message
        message={said({
          kind: "image",
          rendered: "<image omitted: beach.jpg>",
        })}
        language="en"
      />,
    );
    expect(screen.getByText("<image omitted: beach.jpg>")).toBeInTheDocument();
  });

  it("does not repeat itself when the description is the words", () => {
    const { container } = render(
      <Message
        message={said({ text: "hola", rendered: "hola" })}
        language="en"
      />,
    );
    expect(container.textContent.match(/hola/g)).toHaveLength(1);
  });

  it("shows a picture that outlived its file", () => {
    render(
      <Message
        message={said({
          kind: "image",
          attachment: { preview_base64: "AAAA", media_type: "image/jpeg" },
        })}
        language="en"
      />,
    );
    const picture = screen.getByRole("img");
    expect(picture).toHaveAttribute("src", "data:image/jpeg;base64,AAAA");
    expect(screen.getByText(/recovered preview/i)).toBeInTheDocument();
  });

  it("survives a poll whose answers were not recovered", () => {
    // The server sends null here rather than an empty list, which is the one place
    // in the whole API that it does.
    render(
      <Message
        message={said({
          kind: "poll",
          poll: { question: "Where?", options: null },
        })}
        language="en"
      />,
    );
    expect(screen.getByText("Where?")).toBeInTheDocument();
  });

  it("counts a single vote in the singular", () => {
    render(
      <Message
        message={said({
          kind: "poll",
          poll: { question: "Where?", options: [{ name: "Here", votes: 1 }] },
        })}
        language="en"
      />,
    );
    expect(screen.getByText("1 vote")).toBeInTheDocument();
  });

  const tags = [
    {
      name: "edited",
      message: said({ text: "x", edited_at: "2019-06-14T09:13:00Z" }),
      want: "edited",
    },
    {
      name: "forwarded",
      message: said({ text: "x", forwarded: true }),
      want: "forwarded",
    },
    {
      name: "starred",
      message: said({ text: "x", starred: true }),
      want: "starred",
    },
  ] as const;

  it.each(tags)("marks a message that was $name", ({ message, want }) => {
    render(<Message message={message} language="en" />);
    expect(screen.getByText(new RegExp(want))).toBeInTheDocument();
  });

  it("shows a notice without pretending somebody said it", () => {
    const { container } = render(
      <Message
        message={withoutSender({
          kind: "system",
          rendered: "Luis added Marta",
        })}
        language="en"
      />,
    );
    expect(screen.getByText("Luis added Marta")).toBeInTheDocument();
    // A notice carries no attribution, because nobody wrote it.
    expect(container.querySelector("time")).toBeNull();
  });
});
