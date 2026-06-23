import type { ReactNode } from 'react';
import type { MessageEntity } from '../types';

interface FormattedTextProps {
  text: string;
  entities?: MessageEntity[];
  parseMode?: string;
}

interface RichMessageViewProps {
  value: unknown;
}

export function FormattedText({ text, entities, parseMode }: FormattedTextProps) {
  if (!text) {
    return null;
  }
  if ((!entities || entities.length === 0) && parseMode?.toLowerCase() === 'html') {
    return <RichHtmlContent html={text} />;
  }
  return <>{renderEntities(text, entities ?? [])}</>;
}

export function RichMessageView({ value }: RichMessageViewProps) {
  if (value === null || value === undefined) {
    return null;
  }
  return <div className="rich-message">{renderRichMessage(value, 'rich-root')}</div>;
}

function renderEntities(text: string, entities: MessageEntity[]): ReactNode[] {
  const sorted = [...entities]
    .filter((entity) => entity.length > 0 && entity.offset >= 0)
    .sort((a, b) => a.offset - b.offset || b.length - a.length);
  const parts: ReactNode[] = [];
  let cursor = 0;
  sorted.forEach((entity, index) => {
    const start = Math.min(entity.offset, text.length);
    const end = Math.min(start + entity.length, text.length);
    if (start < cursor || end <= start) {
      return;
    }
    if (start > cursor) {
      parts.push(text.slice(cursor, start));
    }
    parts.push(wrapEntity(entity, text.slice(start, end), `entity-${index}`));
    cursor = end;
  });
  if (cursor < text.length) {
    parts.push(text.slice(cursor));
  }
  return parts.length > 0 ? parts : [text];
}

function wrapEntity(entity: MessageEntity, text: string, key: string): ReactNode {
  switch (entity.type) {
    case 'bold':
      return <strong key={key}>{text}</strong>;
    case 'italic':
      return <em key={key}>{text}</em>;
    case 'underline':
      return <span className="formatted-text__underline" key={key}>{text}</span>;
    case 'strikethrough':
      return <s key={key}>{text}</s>;
    case 'spoiler':
      return <span className="formatted-text__spoiler" key={key}>{text}</span>;
    case 'code':
      return <code key={key}>{text}</code>;
    case 'pre':
      return <pre key={key}>{text}</pre>;
    case 'text_link':
      return <SafeLink href={entity.url} key={key}>{text}</SafeLink>;
    case 'url':
      return <SafeLink href={text} key={key}>{text}</SafeLink>;
    case 'custom_emoji':
      return <CustomEmoji id={entity.custom_emoji_id} key={key} text={entity.alternative_text || text} />;
    case 'date_time':
      return <time key={key}>{text}</time>;
    default:
      return <span key={key}>{text}</span>;
  }
}

function RichHtmlContent({ html }: { html: string }) {
  if (typeof DOMParser === 'undefined') {
    return <>{html}</>;
  }
  const doc = new DOMParser().parseFromString(`<body>${html}</body>`, 'text/html');
  return <>{Array.from(doc.body.childNodes).map((node, index) => renderHTMLNode(node, `html-${index}`))}</>;
}

function renderHTMLNode(node: ChildNode, key: string): ReactNode {
  if (node.nodeType === Node.TEXT_NODE) {
    return node.textContent;
  }
  if (node.nodeType !== Node.ELEMENT_NODE) {
    return null;
  }
  const element = node as Element;
  const children = Array.from(element.childNodes).map((child, index) => renderHTMLNode(child, `${key}-${index}`));
  switch (element.tagName.toLowerCase()) {
    case 'b':
    case 'strong':
      return <strong key={key}>{children}</strong>;
    case 'i':
    case 'em':
      return <em key={key}>{children}</em>;
    case 'u':
    case 'ins':
      return <span className="formatted-text__underline" key={key}>{children}</span>;
    case 's':
    case 'strike':
    case 'del':
      return <s key={key}>{children}</s>;
    case 'tg-spoiler':
      return <span className="formatted-text__spoiler" key={key}>{children}</span>;
    case 'code':
      return <code key={key}>{children}</code>;
    case 'pre':
      return <pre key={key}>{children}</pre>;
    case 'a':
      return <SafeLink href={element.getAttribute('href') ?? undefined} key={key}>{children}</SafeLink>;
    case 'tg-emoji':
      return <CustomEmoji id={element.getAttribute('emoji-id') ?? undefined} key={key} text={element.textContent || '✨'} />;
    case 'table':
      return <table className="rich-message__table" key={key}><tbody>{children}</tbody></table>;
    case 'tr':
      return <tr key={key}>{children}</tr>;
    case 'td':
      return <td key={key}>{children}</td>;
    case 'th':
      return <th key={key}>{children}</th>;
    case 'ul':
      return <ul key={key}>{children}</ul>;
    case 'ol':
      return <ol key={key}>{children}</ol>;
    case 'li':
      return <li key={key}>{children}</li>;
    case 'blockquote':
      return <blockquote key={key}>{children}</blockquote>;
    case 'br':
      return <br key={key} />;
    case 'p':
      return <p key={key}>{children}</p>;
    default:
      return <span key={key}>{children}</span>;
  }
}

function renderRichMessage(value: unknown, key: string): ReactNode {
  if (typeof value === 'string' || typeof value === 'number') {
    return String(value);
  }
  if (Array.isArray(value)) {
    return value.map((item, index) => renderRichMessage(item, `${key}-${index}`));
  }
  if (!isRecord(value)) {
    return null;
  }
  if (typeof value.html === 'string') {
    return <RichHtmlContent html={value.html} />;
  }
  if (Array.isArray(value.blocks)) {
    return value.blocks.map((block, index) => renderRichBlock(block, `${key}-block-${index}`));
  }
  if ('text' in value) {
    return renderRichText(value, key);
  }
  return <pre className="rich-message__json">{JSON.stringify(value, null, 2)}</pre>;
}

function renderRichBlock(value: unknown, key: string): ReactNode {
  if (!isRecord(value)) {
    return <p key={key}>{renderRichText(value, `${key}-text`)}</p>;
  }
  const type = String(value.type ?? 'paragraph');
  if (type === 'table') {
    const rows = Array.isArray(value.rows) ? value.rows : Array.isArray(value.cells) ? value.cells : [];
    return (
      <table className="rich-message__table" key={key}>
        <tbody>
          {rows.map((row, rowIndex) => (
            <tr key={`${key}-row-${rowIndex}`}>
              {(Array.isArray(row) ? row : []).map((cell, cellIndex) => (
                <td key={`${key}-cell-${rowIndex}-${cellIndex}`}>{renderRichText(cell, `${key}-cell-text-${rowIndex}-${cellIndex}`)}</td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    );
  }
  if (type === 'section_heading') {
    return <h3 key={key}>{renderRichText(value.text ?? value, `${key}-heading`)}</h3>;
  }
  if (type === 'preformatted') {
    return <pre key={key}>{plainRichText(value.text ?? value)}</pre>;
  }
  if (type === 'divider') {
    return <hr key={key} />;
  }
  if (type === 'list') {
    const items = Array.isArray(value.items) ? value.items : [];
    return <ul key={key}>{items.map((item, index) => <li key={`${key}-item-${index}`}>{renderRichText(item, `${key}-item-text-${index}`)}</li>)}</ul>;
  }
  if (type === 'block_quote' || type === 'pull_quote') {
    return <blockquote key={key}>{renderRichText(value.text ?? value, `${key}-quote`)}</blockquote>;
  }
  if (type === 'thinking') {
    return <p className="rich-message__thinking" key={key}>Думаю...</p>;
  }
  return <p key={key}>{renderRichText(value.text ?? value.caption ?? value, `${key}-paragraph`)}</p>;
}

function renderRichText(value: unknown, key: string): ReactNode {
  if (typeof value === 'string' || typeof value === 'number') {
    return String(value);
  }
  if (Array.isArray(value)) {
    return value.map((item, index) => renderRichText(item, `${key}-${index}`));
  }
  if (!isRecord(value)) {
    return null;
  }
  const type = String(value.type ?? 'text');
  if (type === 'custom_emoji') {
    return <CustomEmoji id={stringField(value, 'custom_emoji_id')} key={key} text={stringField(value, 'alternative_text') || '✨'} />;
  }
  const textValue = value.text ?? value.children ?? value.content ?? '';
  const child = renderRichText(textValue, `${key}-child`);
  switch (type) {
    case 'bold':
      return <strong key={key}>{child}</strong>;
    case 'italic':
      return <em key={key}>{child}</em>;
    case 'underline':
      return <span className="formatted-text__underline" key={key}>{child}</span>;
    case 'strikethrough':
      return <s key={key}>{child}</s>;
    case 'spoiler':
      return <span className="formatted-text__spoiler" key={key}>{child}</span>;
    case 'code':
      return <code key={key}>{child}</code>;
    case 'url':
      return <SafeLink href={stringField(value, 'url')} key={key}>{child}</SafeLink>;
    default:
      return <span key={key}>{child}</span>;
  }
}

function plainRichText(value: unknown): string {
  if (typeof value === 'string' || typeof value === 'number') {
    return String(value);
  }
  if (Array.isArray(value)) {
    return value.map(plainRichText).join('');
  }
  if (isRecord(value)) {
    return plainRichText(value.text ?? value.children ?? value.content ?? value.alternative_text ?? '');
  }
  return '';
}

function SafeLink({ href, children }: { href?: string; children: ReactNode }) {
  const safeHref = href && /^(https?:|tg:|mailto:)/i.test(href) ? href : undefined;
  if (!safeHref) {
    return <span>{children}</span>;
  }
  return <a href={safeHref} rel="noreferrer" target="_blank">{children}</a>;
}

function CustomEmoji({ id, text }: { id?: string; text: string }) {
  return <span className="formatted-text__custom-emoji" title={id ? `custom emoji ${id}` : 'custom emoji'}>{text || '✨'}</span>;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}

function stringField(value: Record<string, unknown>, key: string): string | undefined {
  const field = value[key];
  return typeof field === 'string' ? field : undefined;
}
