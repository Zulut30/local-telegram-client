import type { CSSProperties } from 'react';
import type { Message, ReplyKeyboardButton, ReplyMarkup } from '../types';

interface KeyboardProps {
  message: Message;
  markup?: ReplyMarkup;
  onCallback: (message: Message, data: string) => Promise<void>;
  onReplyText: (text: string) => Promise<void>;
}

function buttonText(button: ReplyKeyboardButton): string {
  return typeof button === 'string' ? button : button.text;
}

type KeyboardRowStyle = CSSProperties & {
  '--keyboard-columns': number;
};

function rowStyle(columns: number): KeyboardRowStyle {
  return { '--keyboard-columns': Math.max(1, columns) };
}

export function Keyboard({ message, markup, onCallback, onReplyText }: KeyboardProps) {
  if (!markup?.inline_keyboard && !markup?.keyboard) {
    return null;
  }

  return (
    <div className="keyboard">
      {markup.inline_keyboard?.map((row, rowIndex) => (
        <div className="keyboard__row" key={`inline-${message.message_id}-${rowIndex}`} style={rowStyle(row.length)}>
          {row.map((button, buttonIndex) => {
            const key = `inline-${rowIndex}-${buttonIndex}`;
            if (button.url) {
              return (
                <a className="keyboard__button" href={button.url} key={key} rel="noreferrer" target="_blank">
                  {button.text}
                </a>
              );
            }
            return (
              <button
                className="keyboard__button"
                key={key}
                type="button"
                onClick={() => onCallback(message, button.callback_data ?? button.text)}
              >
                {button.text}
              </button>
            );
          })}
        </div>
      ))}
      {markup.keyboard?.map((row, rowIndex) => (
        <div className="keyboard__row" key={`reply-${message.message_id}-${rowIndex}`} style={rowStyle(row.length)}>
          {row.map((button, buttonIndex) => (
            <button
              className="keyboard__button keyboard__button--reply"
              key={`reply-${rowIndex}-${buttonIndex}`}
              type="button"
              onClick={() => onReplyText(buttonText(button))}
            >
              {buttonText(button)}
            </button>
          ))}
        </div>
      ))}
    </div>
  );
}
