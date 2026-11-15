/**
 * コミットメッセージの形式: type: :emoji: メッセージ
 *
 * | Type     | Emoji                | 用途                                   |
 * |----------|----------------------|----------------------------------------|
 * | feat     | :sparkles:           | 新機能                                 |
 * | fix      | :bug:                | バグ修正                               |
 * | docs     | :books:              | ドキュメント変更                       |
 * | style    | :art:                | スタイル変更（コードの意味に影響しない）|
 * | refactor | :hammer:             | リファクタリング                       |
 * | perf     | :zap:                | パフォーマンス改善                     |
 * | test     | :white_check_mark:   | テストの追加・修正                     |
 * | build    | :wrench:             | ビルド・ツール・設定の変更             |
 * | ci       | :construction_worker:| CI/CD 設定の変更                       |
 * | chore    | :broom:              | 雑務（依存関係の更新など）             |
 * | revert   | :rewind:             | 変更の取り消し                         |
 *
 * 例: feat: :sparkles: ユーザー認証機能を追加
 */
const typeEmojiMap = {
  feat: ':sparkles:',
  fix: ':bug:',
  docs: ':books:',
  style: ':art:',
  refactor: ':hammer:',
  perf: ':zap:',
  test: ':white_check_mark:',
  build: ':wrench:',
  ci: ':construction_worker:',
  chore: ':broom:',
  revert: ':rewind:',
};

export default {
  parserPreset: {
    parserOpts: {
      // type: :emoji: subject の形式を解析
      headerPattern: /^(\w+): (:[a-z_]+:) (.+)$/,
      headerCorrespondence: ['type', 'emoji', 'subject'],
    },
  },
  rules: {
    // type は必須
    'type-empty': [2, 'never'],
    // type は定義済みのもののみ許可
    'type-enum': [2, 'always', Object.keys(typeEmojiMap)],
    // subject は必須
    'subject-empty': [2, 'never'],
    // header の最大文字数
    'header-max-length': [2, 'always', 120],
    // emoji が type に対応しているか
    'emoji-match': [2, 'always'],
    // Claude のセッションの URL を入れない
    'no-claude-session-url': [2, 'always'],
  },
  plugins: [
    {
      rules: {
        'emoji-match': (parsed) => {
          const { type, emoji } = parsed;
          if (!type || !emoji) {
            return [
              false,
              `コミットメッセージは "type: :emoji: message" の形式にしてください\n` +
              `例: feat: :sparkles: 新機能を追加`,
            ];
          }
          const expectedEmoji = typeEmojiMap[type];
          if (!expectedEmoji) {
            return [true]; // type-enum で別途チェック
          }
          if (emoji !== expectedEmoji) {
            return [
              false,
              `type "${type}" には emoji "${expectedEmoji}" を使用してください (現在: "${emoji}")`,
            ];
          }
          return [true];
        },
        // メッセージ全体（件名・本文・フッター）を見る。大文字小文字は区別しない。
        'no-claude-session-url': (parsed) => {
          const raw = parsed.raw ?? '';
          const found = /claude\.ai\/code\/session/i.test(raw) || /^\s*Claude-Session:/im.test(raw);
          return [
            !found,
            'コミットメッセージに Claude のセッションの URL（Claude-Session: の行）を入れないでください',
          ];
        },
      },
    },
  ],
};
