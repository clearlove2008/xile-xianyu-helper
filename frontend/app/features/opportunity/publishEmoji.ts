/** normalizePublishEmoji 将已确认的平台表情占位码转换为 Unicode；不删除未知括号正文。 */
export const normalizePublishEmoji = (text: string): string => text.replace(
  /\[(右|钉子|[0-9])\]/g,
  (_match, code: string) => {
    if (code === '右') return '➡️';
    if (code === '钉子') return '📌';
    return `${code}\uFE0F\u20E3`;
  },
);
