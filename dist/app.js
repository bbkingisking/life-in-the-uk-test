// Vendored UI library (see vendor/shoelace/README.md for what this is
// and isn't, and how to add another component). Each import below
// self-registers its custom element as a side effect; nothing here is
// called directly. setBasePath() for icon assets happens earlier, in
// index.html's own head module script - see the comment there for why.
import './vendor/shoelace/components/button/button.js';
import './vendor/shoelace/components/button-group/button-group.js';
import './vendor/shoelace/components/badge/badge.js';
import './vendor/shoelace/components/breadcrumb/breadcrumb.js';
import './vendor/shoelace/components/breadcrumb-item/breadcrumb-item.js';
import './vendor/shoelace/components/card/card.js';
import './vendor/shoelace/components/alert/alert.js';
import './vendor/shoelace/components/icon/icon.js';
import './vendor/shoelace/components/icon-button/icon-button.js';
import './vendor/shoelace/components/tag/tag.js';
import './vendor/shoelace/components/radio/radio.js';
import './vendor/shoelace/components/radio-group/radio-group.js';
import './vendor/shoelace/components/checkbox/checkbox.js';
import './vendor/shoelace/components/divider/divider.js';
import './vendor/shoelace/components/progress-bar/progress-bar.js';
import './vendor/shoelace/components/spinner/spinner.js';
import './vendor/shoelace/components/dialog/dialog.js';

// ===================== Theme (light/dark) =====================
// Three states: an explicit user choice (persisted in localStorage) always
// wins; with no explicit choice, the OS-level prefers-color-scheme governs
// via CSS, and this toggle just shows/flips the theme that's currently in
// effect.
const THEME_KEY = 'liuk_theme';
const themeToggleBtn = document.getElementById('theme-toggle');

function systemPrefersDark() {
    return !!(window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches);
}

function getStoredTheme() {
    try {
        return localStorage.getItem(THEME_KEY);
    } catch (error) {
        return null;
    }
}

function setStoredTheme(theme) {
    try {
        localStorage.setItem(THEME_KEY, theme);
    } catch (error) {
        console.error('Failed to save theme preference:', error);
    }
}

function effectiveTheme() {
    return getStoredTheme() || (systemPrefersDark() ? 'dark' : 'light');
}

function updateThemeToggleUI() {
    const isDark = effectiveTheme() === 'dark';
    themeToggleBtn.name = isDark ? 'sun-fill' : 'moon-stars-fill';
    themeToggleBtn.label = isDark ? 'Switch to light mode' : 'Switch to dark mode';
}

function applyTheme(theme) {
    if (theme) {
        document.documentElement.setAttribute('data-theme', theme);
    } else {
        document.documentElement.removeAttribute('data-theme');
    }
    // Shoelace's dark theme (vendor/shoelace/themes/dark.css) is gated by
    // this class rather than a prefers-color-scheme media query, so it
    // needs its own sync to whichever theme is actually in effect.
    document.documentElement.classList.toggle('sl-theme-dark', effectiveTheme() === 'dark');
    updateThemeToggleUI();
}

themeToggleBtn.onclick = () => {
    const next = effectiveTheme() === 'dark' ? 'light' : 'dark';
    setStoredTheme(next);
    applyTheme(next);
};

if (window.matchMedia) {
    window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
        if (!getStoredTheme()) applyTheme(null);
    });
}

applyTheme(getStoredTheme());

// ===================== Persistence Layer (browser-local) =====================
// Progress (per-question stats, the "mistakes" review queue, and the
// "not sure" bookmark list) lives entirely in the browser via localStorage.
// The question content itself is always loaded fresh from exams.json.
const PROGRESS_KEY = 'liuk_progress_v1';

function defaultProgress() {
    return { stats: {}, mistakes: {}, notSure: {} };
}

function loadProgress() {
    try {
        const raw = localStorage.getItem(PROGRESS_KEY);
        if (!raw) return defaultProgress();
        const parsed = JSON.parse(raw);
        return {
            stats: parsed.stats || {},
            mistakes: parsed.mistakes || {},
            notSure: parsed.notSure || {}
        };
    } catch (error) {
        console.error('Failed to read saved progress, starting fresh:', error);
        return defaultProgress();
    }
}

function saveProgress() {
    try {
        localStorage.setItem(PROGRESS_KEY, JSON.stringify(userProgress));
    } catch (error) {
        console.error('Failed to save progress:', error);
    }
}

let userProgress = loadProgress();

// Records the outcome of answering a question. A wrong answer adds the
// question to the "mistakes" review queue; a correct answer resolves it
// out of that queue (it no longer needs review) while keeping its
// lifetime stats intact.
function recordAnswer(uid, wasCorrect) {
    if (!uid) return;
    const stat = userProgress.stats[uid] || { seen: 0, correct: 0, wrong: 0, lastResult: null, lastSeenAt: 0 };
    stat.seen++;
    if (wasCorrect) {
        stat.correct++;
        stat.lastResult = 'correct';
        delete userProgress.mistakes[uid];
    } else {
        stat.wrong++;
        stat.lastResult = 'wrong';
        userProgress.mistakes[uid] = true;
    }
    stat.lastSeenAt = Date.now();
    userProgress.stats[uid] = stat;
    saveProgress();
}

function toggleNotSure(uid) {
    if (userProgress.notSure[uid]) {
        delete userProgress.notSure[uid];
    } else {
        userProgress.notSure[uid] = true;
    }
    saveProgress();
    return !!userProgress.notSure[uid];
}

function isNotSure(uid) {
    return !!userProgress.notSure[uid];
}

// ===================== State Management =====================
let examsData = {};
let categoryMeta = {};
let questionsByUid = {};
let questionsByCategory = {};

let currentQuestions = [];
let currentQuestionIndex = 0;
let score = 0;
let wrongQuestions = [];

// Describes how to restart the current session, and how it should behave.
let currentMode = { type: 'exam', payload: null };
let wasTimed = false;

// Island walk data: fetched lazily (island-walk-pools.json is a multi-MB
// static file - most sessions never open this mode, so it shouldn't
// slow down every page load) and cached in-memory for the rest of the
// session. `currentIslandWalkSteps`, set only by startIslandWalk via
// setupQuiz's optional 4th argument, runs parallel to currentQuestions -
// see renderQuestion's islandwalkVia branch for how the two line up.
let islandPools = null;
let islandPoolsLoadPromise = null;
let currentIslandWalkSteps = null;
let isStoppable = false;

// Timer State
let timerInterval = null;
let timeLeft = 0; // seconds
let startTime = 0;
const EXAM_TIME_LIMIT = 45 * 60; // 45 minutes in seconds

// DOM Elements
const homeView = document.getElementById('home-view');
const sectionView = document.getElementById('section-view');
const quizView = document.getElementById('quiz-view');
const resultView = document.getElementById('result-view');
const examList = document.getElementById('exam-list');
const breadcrumbEl = document.getElementById('breadcrumb');
const stopBtn = document.getElementById('stop-btn');
const timerDisplay = document.getElementById('timer');
const progressEl = document.getElementById('progress');
const progressBar = document.getElementById('progress-bar');
const questionCategoryEl = document.getElementById('question-category');
const questionText = document.getElementById('question-text');
const answersContainer = document.getElementById('answers-container');
const feedbackAlert = document.getElementById('feedback-alert');
const feedbackIcon = document.getElementById('feedback-icon');
const resultStatus = document.getElementById('result-status');
const explanationText = document.getElementById('explanation-text');
const checkBtn = document.getElementById('check-btn');
const nextBtn = document.getElementById('next-btn');
const notSureBtn = document.getElementById('notsure-btn');
const islandWalkLoading = document.getElementById('islandwalk-loading');
const islandWalkIslandsEl = document.getElementById('islandwalk-islands');
const islandWalkStartsEl = document.getElementById('islandwalk-starts');
const islandWalkStartListEl = document.getElementById('islandwalk-start-list');
const islandWalkBackBtn = document.getElementById('islandwalk-back-btn');
const islandWalkViaLine = document.getElementById('islandwalk-via-line');
const islandWalkViaText = document.getElementById('islandwalk-via-text');
const islandWalkBridgeSource = document.getElementById('islandwalk-bridge-source');
const deleteDataBtn = document.getElementById('delete-data-btn');
const deleteDataDialog = document.getElementById('delete-data-dialog');
const deleteDataCancelBtn = document.getElementById('delete-data-cancel-btn');
const deleteDataConfirmBtn = document.getElementById('delete-data-confirm-btn');

// Result View Elements
const scoreText = document.getElementById('score-text');
const scorePercentage = document.getElementById('score-percentage');
const timeTakenText = document.getElementById('time-taken-text');
const timeUpMsg = document.getElementById('time-up-msg');
const wrongAnswersContainer = document.getElementById('wrong-answers-container');
const wrongAnswersList = document.getElementById('wrong-answers-list');
const restartBtn = document.getElementById('restart-btn');
const resultHomeBtn = document.getElementById('result-home-btn');

// Home Mode Buttons
const randomExamBtn = document.getElementById('random-exam-btn');
const marathonBtn = document.getElementById('marathon-btn');

// Category / Review sections
const categoryList = document.getElementById('category-list');
const mistakesList = document.getElementById('mistakes-list');
const mistakesEmpty = document.getElementById('mistakes-empty');
const drillMistakesBtn = document.getElementById('drill-mistakes-btn');
const notsureList = document.getElementById('notsure-list');
const notsureEmpty = document.getElementById('notsure-empty');
const drillNotSureBtn = document.getElementById('drill-notsure-btn');

// Mode-select cards (home view) and the section panels they open
const modeSelectCards = {
    exams: document.getElementById('mode-select-exams'),
    special: document.getElementById('mode-select-special'),
    categories: document.getElementById('mode-select-categories'),
    mistakes: document.getElementById('mode-select-mistakes'),
    notsure: document.getElementById('mode-select-notsure')
};
const mistakesCountBadge = document.getElementById('mistakes-count-badge');
const notsureCountBadge = document.getElementById('notsure-count-badge');

const sectionPanels = {
    exams: document.getElementById('section-exams'),
    special: document.getElementById('section-special'),
    categories: document.getElementById('section-categories'),
    mistakes: document.getElementById('section-mistakes'),
    notsure: document.getElementById('section-notsure'),
    islandwalk: document.getElementById('section-islandwalk')
};
const SECTION_LABELS = {
    exams: 'Numbered exams',
    special: 'Special modes',
    categories: 'Practice by category',
    mistakes: 'Review mistakes',
    notsure: 'Not sure',
    islandwalk: 'Island walk'
};

// Initialization
async function init() {
    examList.innerHTML = '<sl-spinner style="font-size: 2rem;"></sl-spinner>';
    try {
        const response = await fetch('exams.json');
        const data = await response.json();
        examsData = data.exams || data;
        categoryMeta = data.categories || {};

        indexQuestions();
        renderExamList();
        renderCategoryList();
        renderMistakesSection();
        renderNotSureSection();
    } catch (error) {
        console.error('Failed to load exams data:', error);
        examList.innerHTML = `
            <sl-alert variant="danger" open>
                <sl-icon slot="icon" name="exclamation-triangle-fill"></sl-icon>
                Error loading exams. Please ensure exams.json exists.
            </sl-alert>`;
    }
}

// Builds a flat, de-duplicated index of every question (the same question
// can appear verbatim in multiple exams) keyed by its stable uid, plus a
// grouping by topic category for the drill-by-category feature.
function indexQuestions() {
    questionsByUid = {};
    questionsByCategory = {};
    Object.values(examsData).forEach(examQuestions => {
        examQuestions.forEach(q => {
            if (questionsByUid[q.uid]) return;
            questionsByUid[q.uid] = q;
            const cat = q.category || 'uncategorized';
            if (!questionsByCategory[cat]) questionsByCategory[cat] = [];
            questionsByCategory[cat].push(q);
        });
    });
}

function renderExamList() {
    examList.innerHTML = '';
    Object.keys(examsData).forEach(examNum => {
        const btn = document.createElement('sl-button');
        btn.textContent = `Exam ${examNum}`;
        btn.onclick = () => startExam(examNum);
        examList.appendChild(btn);
    });
}

function renderCategoryList() {
    categoryList.innerHTML = '';
    Object.keys(categoryMeta).forEach(slug => {
        const questions = questionsByCategory[slug];
        if (!questions || questions.length === 0) return;

        const btn = document.createElement('sl-button');
        btn.className = 'category-btn';
        btn.appendChild(document.createTextNode(categoryMeta[slug]));

        const count = document.createElement('span');
        count.slot = 'suffix';
        count.className = 'category-count';
        count.textContent = questions.length;
        btn.appendChild(count);

        btn.onclick = () => startCategoryDrill(slug);
        categoryList.appendChild(btn);
    });
}

// Besides filling in the section-view list, keeps the corresponding
// mode-select card's count badge (home view) in sync - both reflect the
// same underlying data, so they're updated from the same place.
function renderMistakesSection() {
    const uids = Object.keys(userProgress.mistakes).filter(uid => questionsByUid[uid]);
    mistakesList.innerHTML = '';

    mistakesCountBadge.textContent = uids.length;
    mistakesCountBadge.classList.toggle('hidden', uids.length === 0);

    if (uids.length === 0) {
        mistakesEmpty.classList.remove('hidden');
        drillMistakesBtn.classList.add('hidden');
        return;
    }

    mistakesEmpty.classList.add('hidden');
    drillMistakesBtn.classList.remove('hidden');

    uids.sort((a, b) => (userProgress.stats[b]?.lastSeenAt || 0) - (userProgress.stats[a]?.lastSeenAt || 0));

    uids.forEach(uid => {
        const question = questionsByUid[uid];
        mistakesList.appendChild(renderListItem(question, userProgress.stats[uid], 'mistakes', () => {
            delete userProgress.mistakes[uid];
            saveProgress();
            renderMistakesSection();
        }));
    });
}

function renderNotSureSection() {
    const uids = Object.keys(userProgress.notSure).filter(uid => questionsByUid[uid]);
    notsureList.innerHTML = '';

    notsureCountBadge.textContent = uids.length;
    notsureCountBadge.classList.toggle('hidden', uids.length === 0);

    if (uids.length === 0) {
        notsureEmpty.classList.remove('hidden');
        drillNotSureBtn.classList.add('hidden');
        return;
    }

    notsureEmpty.classList.add('hidden');
    drillNotSureBtn.classList.remove('hidden');

    uids.forEach(uid => {
        const question = questionsByUid[uid];
        notsureList.appendChild(renderListItem(question, userProgress.stats[uid], 'notsure', () => {
            delete userProgress.notSure[uid];
            saveProgress();
            renderNotSureSection();
        }));
    });
}

// `origin` is the section the item lives in ('mistakes' or 'notsure') -
// startSingleReview needs it to build a breadcrumb that nests the single
// question under the section it was opened from.
function renderListItem(question, stat, origin, onRemove) {
    const card = document.createElement('sl-card');
    card.className = 'list-item';
    card.onclick = () => startSingleReview(question.uid, origin);

    const header = document.createElement('div');
    header.slot = 'header';
    header.className = 'list-item-header';

    const tag = document.createElement('sl-tag');
    tag.size = 'small';
    tag.variant = 'primary';
    tag.textContent = categoryMeta[question.category] || 'Uncategorized';
    header.appendChild(tag);

    const removeBtn = document.createElement('sl-icon-button');
    removeBtn.name = 'x-lg';
    removeBtn.label = 'Remove from this list';
    removeBtn.className = 'list-item-remove';
    removeBtn.onclick = (e) => {
        e.stopPropagation();
        onRemove();
    };
    header.appendChild(removeBtn);
    card.appendChild(header);

    const qText = document.createElement('div');
    qText.className = 'list-item-question';
    qText.textContent = question.question;
    card.appendChild(qText);

    if (stat && stat.seen) {
        const statDiv = document.createElement('div');
        statDiv.slot = 'footer';
        statDiv.className = 'list-item-meta';
        statDiv.textContent = `Seen ${stat.seen}x · ${stat.correct} correct, ${stat.wrong} wrong`;
        card.appendChild(statDiv);
    }

    return card;
}

// Fisher-Yates Shuffle
function shuffle(array) {
    const newArray = [...array];
    for (let i = newArray.length - 1; i > 0; i--) {
        const j = Math.floor(Math.random() * (i + 1));
        [newArray[i], newArray[j]] = [newArray[j], newArray[i]];
    }
    return newArray;
}

function getAllQuestions() {
    let all = [];
    Object.values(examsData).forEach(examQuestions => {
        all = all.concat(examQuestions);
    });
    return all;
}

// ===================== Breadcrumb (header navigation) =====================
// Each crumb is `{ label, onClick }` - `onClick` is omitted for the last
// crumb (it's always the current page, plain context) and for any earlier
// crumb that has no page of its own to jump back to. Now that mode-select
// sections (Numbered exams, Special modes, ...) are real intermediate
// pages rather than just scroll-anchors on Home, every crumb before the
// last one is normally clickable, not just the first.
function setBreadcrumb(items) {
    breadcrumbEl.innerHTML = '';
    if (items.length === 0) {
        breadcrumbEl.classList.add('hidden');
        return;
    }
    items.forEach((item, i) => {
        const el = document.createElement('sl-breadcrumb-item');
        if (i === 0) {
            const icon = document.createElement('sl-icon');
            icon.slot = 'prefix';
            icon.name = 'house-door';
            el.appendChild(icon);
        }
        if (item.onClick) {
            el.classList.add('clickable');
            el.onclick = item.onClick;
        }
        el.appendChild(document.createTextNode(item.label));
        breadcrumbEl.appendChild(el);
    });
    breadcrumbEl.classList.remove('hidden');
}

// ===================== Section View (mode-select drill-down) =====================
// Home is now just five mode-select cards; each opens one of these panels
// instead of everything living on Home at once.
// `breadcrumbItems` defaults to the plain "Home > this section" crumb every
// top-level mode-select card uses; a section reached by a nested path
// instead (Island walk lives inside Special modes, not its own home card)
// passes its own deeper trail - see openIslandWalk.
function showSection(key, breadcrumbItems) {
    // Navigating here away from an in-progress quiz (e.g. via the
    // breadcrumb) abandons it the same way goHome does - otherwise the
    // timer interval and its header badge would keep running unseen.
    clearInterval(timerInterval);
    stopBtn.classList.add('hidden');
    timerDisplay.classList.add('hidden');

    homeView.classList.add('hidden');
    quizView.classList.add('hidden');
    resultView.classList.add('hidden');
    sectionView.classList.remove('hidden');

    Object.keys(sectionPanels).forEach(k => sectionPanels[k].classList.toggle('hidden', k !== key));

    // Reflects any progress made since the panel was last shown.
    if (key === 'mistakes') renderMistakesSection();
    if (key === 'notsure') renderNotSureSection();
    if (key === 'islandwalk') renderIslandWalkSection();

    setBreadcrumb(breadcrumbItems || [
        { label: 'Home', onClick: goHome },
        { label: SECTION_LABELS[key] }
    ]);
    window.scrollTo(0, 0);
}

// Island walk lives inside Special modes now, not as its own home card -
// reached via the "Start" button on its mode-card there (see
// islandWalkOpenBtn below) and, from inside a walk, via breadcrumb
// crumbs that need to rebuild the same nested trail (see startIslandWalk).
function openIslandWalk() {
    showSection('islandwalk', [
        { label: 'Home', onClick: goHome },
        { label: SECTION_LABELS.special, onClick: () => showSection('special') },
        { label: SECTION_LABELS.islandwalk }
    ]);
}

// ===================== Session Starters =====================
// Every starter sets `currentMode` (so "Try Again" can restart the same
// kind of session) and calls setupQuiz with how the session should behave:
// `timed` shows the 45-minute countdown, `stoppable` shows a Stop button
// and scores only the questions actually answered (for open-ended drills).

function startExam(examNum) {
    currentMode = { type: 'exam', payload: examNum };
    setupQuiz(shuffle(examsData[examNum]), [
        { label: 'Home', onClick: goHome },
        { label: SECTION_LABELS.exams, onClick: () => showSection('exams') },
        { label: `Exam ${examNum}` }
    ], { timed: true, stoppable: false });
}

function startRandomExam() {
    currentMode = { type: 'random' };
    const all = getAllQuestions();
    const randomSelection = shuffle(all).slice(0, 24);
    setupQuiz(randomSelection, [
        { label: 'Home', onClick: goHome },
        { label: SECTION_LABELS.special, onClick: () => showSection('special') },
        { label: 'Random exam' }
    ], { timed: true, stoppable: false });
}

function startMarathon() {
    currentMode = { type: 'marathon' };
    const all = getAllQuestions();
    setupQuiz(shuffle(all), [
        { label: 'Home', onClick: goHome },
        { label: SECTION_LABELS.special, onClick: () => showSection('special') },
        { label: 'Marathon exam' }
    ], { timed: false, stoppable: true });
}

function startCategoryDrill(slug) {
    const pool = questionsByCategory[slug];
    if (!pool || pool.length === 0) return;
    currentMode = { type: 'category', payload: slug };
    setupQuiz(shuffle(pool), [
        { label: 'Home', onClick: goHome },
        { label: SECTION_LABELS.categories, onClick: () => showSection('categories') },
        { label: categoryMeta[slug] || 'Category drill' }
    ], { timed: false, stoppable: true });
}

function startMistakesDrill() {
    const pool = Object.keys(userProgress.mistakes)
        .map(uid => questionsByUid[uid])
        .filter(Boolean);
    if (pool.length === 0) return;
    currentMode = { type: 'mistakes' };
    setupQuiz(shuffle(pool), [
        { label: 'Home', onClick: goHome },
        { label: SECTION_LABELS.mistakes, onClick: () => showSection('mistakes') },
        { label: 'Drill all' }
    ], { timed: false, stoppable: true });
}

function startNotSureDrill() {
    const pool = Object.keys(userProgress.notSure)
        .map(uid => questionsByUid[uid])
        .filter(Boolean);
    if (pool.length === 0) return;
    currentMode = { type: 'notsure' };
    setupQuiz(shuffle(pool), [
        { label: 'Home', onClick: goHome },
        { label: SECTION_LABELS.notsure, onClick: () => showSection('notsure') },
        { label: 'Drill all' }
    ], { timed: false, stoppable: true });
}

// `origin` ('mistakes' or 'notsure', or omitted) says which section list
// this single question was opened from, so both the breadcrumb and "Try
// Again" (which replays via currentMode.origin) can nest it correctly.
function startSingleReview(uid, origin) {
    const question = questionsByUid[uid];
    if (!question) return;
    currentMode = { type: 'single', payload: uid, origin };
    const items = [{ label: 'Home', onClick: goHome }];
    if (origin === 'mistakes' || origin === 'notsure') {
        items.push({ label: SECTION_LABELS[origin], onClick: () => showSection(origin) });
    }
    items.push({ label: 'Reviewing question' });
    setupQuiz([question], items, { timed: false, stoppable: false });
}

// ===================== Island walk =====================
// See ../docs/KNOWLEDGE-WALK-DESIGN.md. island-walk-pools.json holds, per island
// (a connected cluster of the Knowledge Map with enough question content
// to walk), a pool of pre-verified traversals - each a strict-adjacency
// sequence of question uids, resolved here against questionsByUid rather
// than duplicating question content in the pool file.
function ensureIslandPoolsLoaded() {
    if (islandPools) return Promise.resolve(islandPools);
    if (!islandPoolsLoadPromise) {
        islandPoolsLoadPromise = fetch('island-walk-pools.json')
            .then(response => response.json())
            .then(data => {
                islandPools = data;
                return data;
            });
    }
    return islandPoolsLoadPromise;
}

async function renderIslandWalkSection() {
    islandWalkIslandsEl.classList.add('hidden');
    islandWalkStartsEl.classList.add('hidden');
    islandWalkLoading.classList.remove('hidden');
    try {
        await ensureIslandPoolsLoaded();
    } catch (error) {
        console.error('Failed to load Island walk data:', error);
        islandWalkLoading.classList.add('hidden');
        islandWalkIslandsEl.innerHTML = `
            <sl-alert variant="danger" open>
                <sl-icon slot="icon" name="exclamation-triangle-fill"></sl-icon>
                Could not load Island walk data. Please ensure island-walk-pools.json exists.
            </sl-alert>`;
        islandWalkIslandsEl.classList.remove('hidden');
        return;
    }
    islandWalkLoading.classList.add('hidden');
    renderIslandList();
}

function renderIslandList() {
    islandWalkStartsEl.classList.add('hidden');
    islandWalkIslandsEl.innerHTML = '';
    islandPools.islands.forEach(island => {
        const btn = document.createElement('sl-button');
        btn.className = 'category-btn';
        btn.appendChild(document.createTextNode(island.label));
        btn.onclick = () => renderStartChoices(island);
        islandWalkIslandsEl.appendChild(btn);
    });
    islandWalkIslandsEl.classList.remove('hidden');
}

// Offers 4 starting points with distinct topics, drawn at random from the
// island's pool - so repeat visits to the same island can offer a
// different 4 each time (see ../docs/KNOWLEDGE-WALK-DESIGN.md's "pick your
// island" UX, settled after confirming the pool comfortably covers
// hundreds of distinct starts, not just 4).
function renderStartChoices(island) {
    islandWalkIslandsEl.classList.add('hidden');
    islandWalkStartListEl.innerHTML = '';

    const seenStarts = new Set();
    const choices = [];
    for (const traversal of shuffle(island.traversals)) {
        if (seenStarts.has(traversal.start)) continue;
        seenStarts.add(traversal.start);
        choices.push(traversal);
        if (choices.length === 4) break;
    }

    choices.forEach(traversal => {
        const btn = document.createElement('sl-button');
        btn.className = 'category-btn';
        btn.appendChild(document.createTextNode(traversal.start));
        btn.onclick = () => startIslandWalk(island, traversal);
        islandWalkStartListEl.appendChild(btn);
    });

    islandWalkStartsEl.classList.remove('hidden');
}

// Resolves the traversal's question uids against questionsByUid (already
// loaded from exams.json - the pool file never duplicates question
// content) and starts it as an untimed, stoppable session, same shape as
// Marathon/Category drill. Order is never shuffled: it's the whole point
// of the mode that each question is adjacent to the one before it.
function startIslandWalk(island, traversal) {
    const questions = [];
    const steps = [];
    traversal.steps.forEach(step => {
        const q = questionsByUid[step.q];
        if (!q) return; // defensive: pool and exams.json should always agree
        questions.push(q);
        steps.push(step);
    });
    if (questions.length === 0) return;

    currentMode = { type: 'islandwalk', payload: { island, traversal } };
    setupQuiz(questions, [
        { label: 'Home', onClick: goHome },
        { label: SECTION_LABELS.special, onClick: () => showSection('special') },
        { label: SECTION_LABELS.islandwalk, onClick: openIslandWalk },
        { label: island.label, onClick: () => renderStartChoices(island) },
        { label: `Starting at ${traversal.start}` }
    ], { timed: false, stoppable: true }, steps);
}

islandWalkBackBtn.onclick = () => renderIslandList();
document.getElementById('islandwalk-open-btn').onclick = openIslandWalk;

function setupQuiz(questions, breadcrumbItems, options, islandWalkSteps) {
    const { timed = false, stoppable = false } = options || {};
    clearInterval(timerInterval);
    currentQuestions = questions;
    currentQuestionIndex = 0;
    score = 0;
    wrongQuestions = [];
    wasTimed = timed;
    isStoppable = stoppable;
    currentIslandWalkSteps = islandWalkSteps || null;

    homeView.classList.add('hidden');
    sectionView.classList.add('hidden');
    resultView.classList.add('hidden');
    quizView.classList.remove('hidden');

    if (stoppable) {
        stopBtn.classList.remove('hidden');
    } else {
        stopBtn.classList.add('hidden');
    }

    if (timed) {
        startTimer();
    } else {
        timerDisplay.classList.add('hidden');
    }

    setBreadcrumb(breadcrumbItems);
    renderQuestion();
    window.scrollTo(0, 0);
}

function startTimer() {
    timeLeft = EXAM_TIME_LIMIT;
    startTime = Date.now();
    timerDisplay.classList.remove('hidden');
    timerDisplay.variant = 'neutral';
    updateTimerDisplay();

    timerInterval = setInterval(() => {
        timeLeft--;
        updateTimerDisplay();

        if (timeLeft <= 60) {
            timerDisplay.variant = 'danger';
        }

        if (timeLeft <= 0) {
            clearInterval(timerInterval);
            showResults(true);
        }
    }, 1000);
}

function updateTimerDisplay() {
    const mins = Math.floor(timeLeft / 60);
    const secs = timeLeft % 60;
    timerDisplay.textContent = `${mins.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`;
}

// Every answer control (radio or checkbox) ends up here regardless of
// which the question needed, so checking "what's selected" never has to
// branch on question type.
function getAnswerInputs() {
    return [...answersContainer.querySelectorAll('sl-radio, sl-checkbox')];
}

// Shows/hides the "connected via X" line above the question text -
// visible only during an island walk session (currentIslandWalkSteps is
// set only by startIslandWalk, via setupQuiz's optional 4th argument, and
// runs parallel to currentQuestions). Built with DOM methods rather than
// innerHTML so the entity label never needs escaping.
function renderIslandWalkVia() {
    const step = currentIslandWalkSteps ? currentIslandWalkSteps[currentQuestionIndex] : null;
    if (!step) {
        islandWalkViaLine.classList.add('hidden');
        return;
    }

    islandWalkViaText.textContent = '';
    islandWalkViaText.appendChild(document.createTextNode(
        currentQuestionIndex === 0 ? 'Starting topic: ' : '↳ connected via '
    ));
    const b = document.createElement('b');
    b.textContent = step.via;
    islandWalkViaText.appendChild(b);

    // Collapsed by default - the citation is supporting evidence for
    // anyone who wants to check it, not something to force into view on
    // every single step (most of them; see ../docs/KNOWLEDGE-WALK-DESIGN.md - 81%
    // of transitions are this "adjacency-only" kind).
    islandWalkBridgeSource.classList.add('hidden');
    const bridgeQuestion = step.bridgeSourceUid && questionsByUid[step.bridgeSourceUid];
    if (bridgeQuestion) {
        islandWalkBridgeSource.textContent = `both appear together in: "${bridgeQuestion.question}"`;

        islandWalkViaText.appendChild(document.createTextNode(' '));
        const seeHowBtn = document.createElement('button');
        seeHowBtn.type = 'button';
        seeHowBtn.className = 'link-btn';
        seeHowBtn.textContent = '(see how)';
        seeHowBtn.onclick = () => islandWalkBridgeSource.classList.toggle('hidden');
        islandWalkViaText.appendChild(seeHowBtn);
    }

    islandWalkViaLine.classList.remove('hidden');
}

function renderQuestion() {
    const question = currentQuestions[currentQuestionIndex];
    const shuffledAnswers = shuffle(question.answers);

    progressEl.textContent = `Question ${currentQuestionIndex + 1} of ${currentQuestions.length}`;
    progressBar.value = (currentQuestionIndex / currentQuestions.length) * 100;
    questionText.textContent = question.question;
    answersContainer.innerHTML = '';

    if (question.category && categoryMeta[question.category]) {
        questionCategoryEl.textContent = categoryMeta[question.category];
        questionCategoryEl.classList.remove('hidden');
    } else {
        questionCategoryEl.classList.add('hidden');
    }

    renderIslandWalkVia();

    notSureBtn.name = isNotSure(question.uid) ? 'bookmark-fill' : 'bookmark';
    notSureBtn.classList.toggle('active', isNotSure(question.uid));

    feedbackAlert.open = false;
    checkBtn.classList.remove('hidden');
    checkBtn.disabled = true;
    nextBtn.classList.add('hidden');

    // A question with more than one correct answer needs independent
    // checkboxes; exactly one correct answer gets a real radio group, so
    // the browser (and Shoelace) enforce "just one" for free.
    const correctCount = question.answers.filter(a => a.isCorrect).length;
    const isMultiple = correctCount > 1;

    let container = answersContainer;
    if (!isMultiple) {
        container = document.createElement('sl-radio-group');
        container.setAttribute('aria-label', question.question);
        answersContainer.appendChild(container);
    }

    shuffledAnswers.forEach((ans, idx) => {
        const el = document.createElement(isMultiple ? 'sl-checkbox' : 'sl-radio');
        el.className = 'answer-option';
        el.dataset.isCorrect = ans.isCorrect;
        if (!isMultiple) el.value = String(idx);
        el.textContent = ans.text;
        container.appendChild(el);
    });
}

function checkAnswer() {
    const question = currentQuestions[currentQuestionIndex];
    const inputs = getAnswerInputs();
    let allCorrect = true;
    let anyWrong = false;

    inputs.forEach(el => {
        const isCorrect = el.dataset.isCorrect === 'true';
        const isChecked = el.checked;

        if (isCorrect) {
            el.classList.add('correct');
            if (!isChecked) allCorrect = false;
        } else if (isChecked) {
            el.classList.add('incorrect');
            anyWrong = true;
        }
        el.disabled = true;
    });

    const success = allCorrect && !anyWrong;
    if (success) {
        score++;
    } else {
        wrongQuestions.push({
            question: question.question,
            correctAnswer: question.answers.filter(a => a.isCorrect).map(a => a.text).join(', '),
            explanation: question.reference
        });
    }

    recordAnswer(question.uid, success);

    feedbackAlert.variant = success ? 'success' : 'danger';
    feedbackIcon.name = success ? 'check-circle' : 'exclamation-triangle-fill';
    resultStatus.textContent = success ? 'Correct!' : 'Incorrect';
    explanationText.textContent = question.reference || 'No explanation available.';
    feedbackAlert.open = true;

    checkBtn.classList.add('hidden');
    nextBtn.classList.remove('hidden');

    feedbackAlert.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
}

function nextQuestion() {
    currentQuestionIndex++;
    if (currentQuestionIndex < currentQuestions.length) {
        renderQuestion();
        window.scrollTo(0, 0);
    } else {
        showResults();
    }
}

function showResults(isTimeUp = false) {
    clearInterval(timerInterval);
    quizView.classList.add('hidden');
    stopBtn.classList.add('hidden');
    timerDisplay.classList.add('hidden');
    resultView.classList.remove('hidden');
    setBreadcrumb([{ label: 'Home', onClick: goHome }, { label: 'Results' }]);

    timeUpMsg.open = isTimeUp;

    // Stoppable (untimed, open-ended) sessions may be cut short, so the
    // total only counts questions actually answered.
    const finalTotal = isStoppable ? (score + wrongQuestions.length) : currentQuestions.length;

    const percentage = Math.round((score / finalTotal) * 100) || 0;

    scoreText.textContent = `Your score: ${score}/${finalTotal}`;
    scorePercentage.textContent = `${percentage}%`;

    if (wasTimed) {
        const timeSpent = Math.floor((Date.now() - startTime) / 1000);
        const spentMins = Math.floor(timeSpent / 60);
        const spentSecs = timeSpent % 60;
        timeTakenText.textContent = `Time taken: ${spentMins.toString().padStart(2, '0')}:${spentSecs.toString().padStart(2, '0')}`;
        timeTakenText.classList.remove('hidden');
    } else {
        timeTakenText.classList.add('hidden');
    }

    if (wrongQuestions.length > 0) {
        wrongAnswersContainer.classList.remove('hidden');
        wrongAnswersList.innerHTML = '';
        wrongQuestions.forEach(item => {
            const card = document.createElement('sl-card');
            card.className = 'wrong-item';
            card.innerHTML = `
                <div slot="header" class="wrong-question">${item.question}</div>
                <div class="correct-answer-was">Correct: ${item.correctAnswer}</div>
                <div class="wrong-explanation">${item.explanation || ''}</div>
            `;
            wrongAnswersList.appendChild(card);
        });
    } else {
        wrongAnswersContainer.classList.add('hidden');
    }

    window.scrollTo(0, 0);
}

function goHome() {
    clearInterval(timerInterval);
    homeView.classList.remove('hidden');
    sectionView.classList.add('hidden');
    quizView.classList.add('hidden');
    resultView.classList.add('hidden');
    stopBtn.classList.add('hidden');
    timerDisplay.classList.add('hidden');
    setBreadcrumb([]);

    // Stats may have changed during the session, so refresh the lists
    // (and the mode-select cards' count badges, updated from the same
    // place - see renderMistakesSection/renderNotSureSection).
    renderMistakesSection();
    renderNotSureSection();

    window.scrollTo(0, 0);
}

function restartExam() {
    switch (currentMode.type) {
        case 'exam': startExam(currentMode.payload); break;
        case 'random': startRandomExam(); break;
        case 'marathon': startMarathon(); break;
        case 'category': startCategoryDrill(currentMode.payload); break;
        case 'mistakes': startMistakesDrill(); break;
        case 'notsure': startNotSureDrill(); break;
        case 'single': startSingleReview(currentMode.payload, currentMode.origin); break;
        case 'islandwalk': startIslandWalk(currentMode.payload.island, currentMode.payload.traversal); break;
        default: goHome();
    }
}

// Event Listeners
Object.keys(modeSelectCards).forEach(key => {
    const card = modeSelectCards[key];
    card.onclick = () => showSection(key);
    // sl-card isn't natively a button - role="button"/tabindex in the
    // markup make it focusable and announce as one, this makes Enter/Space
    // actually activate it, matching native button behavior.
    card.addEventListener('keydown', (e) => {
        if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            showSection(key);
        }
    });
});

// Knowledge Map card: a real navigation (its own static page), not a
// mode-select-card in the map above - there's no section-view panel to
// open for it, so it's wired separately rather than folded into the
// showSection() loop.
const knowledgeMapCard = document.getElementById('mode-select-knowledge-map');
knowledgeMapCard.onclick = () => { window.location.href = 'knowledge-map.html'; };
knowledgeMapCard.addEventListener('keydown', (e) => {
    if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        window.location.href = 'knowledge-map.html';
    }
});
checkBtn.onclick = checkAnswer;
nextBtn.onclick = nextQuestion;
stopBtn.onclick = () => showResults();
randomExamBtn.onclick = startRandomExam;
marathonBtn.onclick = startMarathon;
restartBtn.onclick = restartExam;
resultHomeBtn.onclick = goHome;
drillMistakesBtn.onclick = startMistakesDrill;
drillNotSureBtn.onclick = startNotSureDrill;
notSureBtn.onclick = () => {
    const question = currentQuestions[currentQuestionIndex];
    if (!question) return;
    const active = toggleNotSure(question.uid);
    notSureBtn.name = active ? 'bookmark-fill' : 'bookmark';
    notSureBtn.classList.toggle('active', active);
};
deleteDataBtn.onclick = () => deleteDataDialog.show();
deleteDataCancelBtn.onclick = () => deleteDataDialog.hide();
deleteDataConfirmBtn.onclick = () => {
    // Wipes this origin's localStorage outright rather than removing
    // THEME_KEY/PROGRESS_KEY individually - "delete everything" should
    // stay correct even if a future key is added here and forgotten
    // there. Reloading afterward is the simplest way to get every piece
    // of in-memory state (theme, current view, progress) back in sync
    // with the now-empty storage, same as a first-ever visit.
    localStorage.clear();
    location.reload();
};

// One delegated listener covers every answer control on the page,
// radio or checkbox, present now or re-rendered for the next question.
answersContainer.addEventListener('sl-change', () => {
    const inputs = getAnswerInputs();
    inputs.forEach(el => el.classList.toggle('selected', el.checked));
    checkBtn.disabled = inputs.filter(el => el.checked).length === 0;
});

// Start the app
init();
