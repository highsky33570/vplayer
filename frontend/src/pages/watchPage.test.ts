import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { describe, it } from 'node:test';
import { fileURLToPath } from 'node:url';
import {
  commentValidationMessage,
  recommendationMeta,
  showEpisodeSelector,
} from './watchPageLogic.ts';

const here = dirname(fileURLToPath(import.meta.url));
const read = (path: string) => readFileSync(path, 'utf8');
const header = read(join(here, '../components/Header.tsx'));
const watch = read(join(here, 'Watch.tsx'));
const comments = read(join(here, '../components/CommentSection.tsx'));
const recs = read(join(here, '../components/RecommendationList.tsx'));
const episodes = read(join(here, '../components/EpisodeStrip.tsx'));
const css = read(join(here, '../styles.css'));

describe('watch page helpers', () => {
  it('hides the episode selector unless there is more than one episode', () => {
    assert.equal(showEpisodeSelector(0), false);
    assert.equal(showEpisodeSelector(1), false);
    assert.equal(showEpisodeSelector(2), true);
  });

  it('rejects blank and over-long comments and accepts 1000 runes', () => {
    assert.equal(commentValidationMessage('  \n '), '评论不能为空');
    assert.equal(commentValidationMessage('你'.repeat(1001)), '评论不能超过 1000 字');
    assert.equal(commentValidationMessage('你'.repeat(1000)), null);
    assert.equal(commentValidationMessage('  hello  '), null);
  });

  it('builds a compact recommendation meta line', () => {
    assert.equal(recommendationMeta(['12 播放', '', '2024', '电影']), '12 播放 · 2024 · 电影');
  });
});

describe('header logged-in presentation', () => {
  it('puts logout before the avatar and does not print a name under it', () => {
    const start = header.indexOf('banner-user-menu');
    const menu = header.slice(start, start + 700);
    const logoutAt = menu.indexOf('className="banner-logout"');
    const avatarAt = menu.indexOf('className="banner-user is-logged"');
    assert.ok(logoutAt >= 0 && avatarAt > logoutAt);
    assert.equal(menu.includes("{user.nickname || '已登录'}"), false);
    assert.equal(menu.includes('title={user.email}'), false);
    assert.match(menu, /已登录/);
    assert.match(header, /退出登录/);
    assert.match(header, /onClick=\{logout\}/);
  });
});

describe('public watch page', () => {
  it('lets guests read the page and keeps playback behind login', () => {
    assert.equal(watch.includes('guests go to login'), false);
    const guard = watch.indexOf('if (!videoId || !ready || !user || !token)');
    const guardBody = watch.slice(guard, guard + 90);
    assert.match(guardBody, /return;/);
    assert.doesNotMatch(guardBody, /navigate/);
    assert.match(watch, /登录后观看完整视频/);
    assert.match(watch, /立即登录/);
    assert.match(watch, /loginWithNext\(`\/watch\/\$\{videoId\}`\)/);
    const play = watch.indexOf('getPlayback(videoId, sid, nid, token)');
    assert.ok(guard >= 0 && play > guard);
    assert.equal(watch.split('getPlayback(').length, 2);
    assert.match(watch, /WatchPlaybackSession/);
    assert.match(watch, /session\.cancel\(\)/);
  });

  it('resets playback selection and scrolls when the video changes', () => {
    assert.match(watch, /setSid\(0\)/);
    assert.match(watch, /setNid\(0\)/);
    assert.match(watch, /window\.scrollTo/);
    assert.match(watch, /key=\{videoId\}/);
  });

  it('renders recommendations, episodes, and the desktop grid hook', () => {
    assert.match(recs, /to=\{`\/watch\/\$\{item\.id\}`\}/);
    assert.match(recs, /推荐视频/);
    assert.match(watch, /watch-main-grid/);
    assert.match(watch, /has-recs/);
    assert.match(episodes, /showEpisodeSelector/);
    assert.match(watch, /setSid\(episode\.sid\)/);
    assert.match(watch, /setNid\(episode\.nid\)/);
    assert.match(css, /@media \(min-width: 1280px\)/);
    assert.match(css, /grid-template-columns: minmax\(0, 3fr\) minmax\(220px, 1fr\)/);
    assert.match(css, /white-space: pre-wrap/);
    assert.match(css, /overflow-wrap: anywhere/);
  });
});

describe('comment section', () => {
  it('covers guest, composer, errors, load more, and safe text', () => {
    assert.match(comments, /<h3>评论<\/h3>/);
    assert.match(comments, /原站评论 \{sourceTotal\}/);
    assert.match(comments, /VPlayer 评论 \{total\}/);
    assert.match(comments, /登录后发表评论/);
    assert.match(comments, /暂无评论，来说两句吧/);
    assert.match(comments, /暂无原站评论/);
    assert.match(comments, /原站评论加载中…/);
    assert.match(comments, /原站评论加载失败/);
    assert.match(comments, /评论加载失败/);
    assert.match(comments, /重试/);
    assert.match(comments, /加载更多原站评论/);
    assert.match(comments, /加载更多/);
    assert.match(comments, /发布中…/);
    assert.match(comments, /disabled=\{posting\}/);
    assert.match(comments, /window\.confirm\('确认删除这条评论？'\)/);
    assert.match(comments, /\{comment\.body\}/);
    assert.match(comments, /\{reply\.body\}/);
    assert.match(comments, /watch-comment-reply/);
    assert.match(comments, /watch-source-badge/);
    assert.match(comments, /sourceCommentTime\(comment\.source_created_raw\)/);
    assert.match(comments, /sourceCommentTime\(reply\.source_created_raw\)/);
    assert.equal(comments.includes('formatCommentTime(comment.source_created_raw)'), false);
    assert.equal(comments.includes('dangerouslySetInnerHTML'), false);
    assert.equal(comments.includes('<img'), false);
    assert.match(comments, /postComment\(videoId, draft\.trim\(\), token\)/);
    assert.match(comments, /reloadFirstPage/);
    assert.equal(comments.includes('user_id'), false);
    const sourceStart = comments.indexOf('function SourceThreads');
    const nativeStart = comments.indexOf('function NativeComments');
    const sourceFn = comments.slice(sourceStart, nativeStart);
    assert.equal(sourceFn.includes('删除'), false);
    assert.match(comments.slice(nativeStart), /删除/);
    assert.match(sourceFn, /原站/);
  });
});
