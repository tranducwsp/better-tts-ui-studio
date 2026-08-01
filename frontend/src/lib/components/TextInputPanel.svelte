<script lang="ts">
  import type { InputPanelSpec, UniversalManifest } from '../types';
  import { resolveChunkSize } from '../textLimits';
  import { extractTextFromFile } from '../api';
  import { toast } from '../toast.svelte';

  interface Props {
    text: string;
    isReadOnly?: boolean;
    inputPanelSpec?: InputPanelSpec | null;
    manifest?: UniversalManifest | null;
  }

  let { text = $bindable(), isReadOnly = $bindable(false), inputPanelSpec = null, manifest = null }: Props = $props();

  let isCollapsed = $state(false);
  let isRegexMode = $state(false);
  let findQuery = $state('');
  let replaceQuery = $state('');
  let uploadedFileName = $state('');
  let lastSearchIndex = $state(0);
  let textareaElement = $state<HTMLTextAreaElement | undefined>();

  // Shared resolver: the preview must match what StreamingPanel actually sends.
  let maxChunkSize = $derived(resolveChunkSize(manifest));
  let chunkDelimiters = $derived(inputPanelSpec?.chunk_delimiters || ['(?<=\\.\\s*\\n)', '[^.!?]+[.!?]+']);

  // Svelte 5 derived state for text chunks (Supports custom regex delimiters from manifest)
  let chunks = $derived.by(() => {
    const limit = maxChunkSize;
    if (!text || text.length <= limit) return text ? [text] : [];

    let paragraphRegex: RegExp;
    try {
      paragraphRegex = new RegExp(chunkDelimiters[0] || '\\n\\n');
    } catch {
      paragraphRegex = /\n\n/;
    }

    const paragraphs = text.split(paragraphRegex);
    const result: string[] = [];
    let currentChunk = '';

    for (const p of paragraphs) {
      const cleanP = p.trim();
      if (!cleanP) continue;

      if (cleanP.length > limit * 2) {
        let sentenceRegex: RegExp;
        try {
          sentenceRegex = new RegExp(chunkDelimiters[1] || '[^.!?]+[.!?]+', 'g');
        } catch {
          sentenceRegex = /[^.!?]+[.!?]+/g;
        }
        const sentences = cleanP.match(sentenceRegex) || [cleanP];
        for (const s of sentences) {
          const cleanS = s.trim();
          if (!cleanS) continue;
          if (currentChunk.length + cleanS.length + 1 <= limit) {
            currentChunk += (currentChunk ? ' ' : '') + cleanS;
          } else {
            if (currentChunk) result.push(currentChunk);
            currentChunk = cleanS;
          }
        }
      } else {
        if (currentChunk.length + cleanP.length + 1 <= limit) {
          currentChunk += (currentChunk ? '\n' : '') + cleanP;
        } else {
          if (currentChunk) result.push(currentChunk);
          currentChunk = cleanP;
        }
      }
    }
    if (currentChunk) result.push(currentChunk);
    return result.length ? result : [text];
  });

  let fileInput = $state<HTMLInputElement | undefined>();

  async function handleFileUpload(e: Event) {
    const target = e.target as HTMLInputElement;
    if (!target.files?.length) return;
    const file = target.files[0];
    toast.show('Reading document...', 'info');
    try {
      const extracted = await extractTextFromFile(file);
      if (extracted) {
        text = extracted;
        isReadOnly = false;
        uploadedFileName = file.name;
        runAutoFormat(false);
        toast.show('Document loaded successfully!', 'success');
      }
    } catch (err: unknown) {
      const errMsg = err instanceof Error ? err.message : String(err);
      toast.show('Error reading file: ' + errMsg, 'error');
    }
    target.value = ''; // Reset input
  }

  function runAutoFormat(notify = false) {
    if (!text || isReadOnly) return;
    let cleaned = text;

    const rules = inputPanelSpec?.auto_format || [
      { find: '\\r\\n', replace: '\n' },
      { find: '\\n{3,}', replace: '\n\n' },
      { find: '\\u00D0', replace: '\u0110' },
      { find: '([a-zA-ZÀ-ỹ])\\-([a-zA-ZÀ-ỹ])', replace: '$1 $2' },
      { find: '[⁰¹²³⁴⁵⁶⁷⁸⁹₀₁₂₃₄₅₆₇₈₉]', replace: '' },
      { find: '[^a-zA-Z0-9 \\n\\t\\r.,?!;:\\-\"\'()\\[\\]%/“”‘’À-ỹ]', replace: '' }
    ];

    for (const rule of rules) {
      try {
        const regex = new RegExp(rule.find, 'g');
        cleaned = cleaned.replace(regex, rule.replace);
      } catch (err) {
        console.error('Auto format rule error:', rule, err);
      }
    }

    cleaned = cleaned.trim();
    if (cleaned !== text) {
      text = cleaned;
      if (notify) {
        toast.show('Text cleaned successfully!', 'success');
      }
    } else if (notify) {
      toast.show('Text is already clean!', 'info');
    }
  }

  function toggleRegexMode() {
    isRegexMode = !isRegexMode;
    lastSearchIndex = 0;
  }

  function getScrollPositionOfIndex(element: HTMLTextAreaElement, position: number): number {
    if (!element) return 0;
    const textBefore = element.value.substring(0, position);
    const computedStyle = window.getComputedStyle(element);

    const div = document.createElement('div');
    const stylesToCopy: string[] = [
      'direction',
      'fontStyle', 'fontVariant', 'fontWeight', 'fontStretch', 'fontSize',
      'lineHeight', 'fontFamily', 'textAlign', 'textTransform', 'textIndent',
      'textDecoration', 'letterSpacing', 'wordSpacing', 'tabSize',
      'whiteSpace', 'wordBreak', 'overflowWrap'
    ];

    div.style.position = 'absolute';
    div.style.visibility = 'hidden';
    div.style.top = '-9999px';
    div.style.left = '-9999px';
    div.style.boxSizing = 'border-box';
    div.style.width = `${element.clientWidth}px`;
    div.style.paddingTop = computedStyle.paddingTop;
    div.style.paddingRight = computedStyle.paddingRight;
    div.style.paddingBottom = computedStyle.paddingBottom;
    div.style.paddingLeft = computedStyle.paddingLeft;
    div.style.borderTopWidth = computedStyle.borderTopWidth;
    div.style.borderRightWidth = computedStyle.borderRightWidth;
    div.style.borderBottomWidth = computedStyle.borderBottomWidth;
    div.style.borderLeftWidth = computedStyle.borderLeftWidth;

      stylesToCopy.forEach((prop) => {
        (div.style as unknown as Record<string, string>)[prop] = (computedStyle as unknown as Record<string, string>)[prop];
      });

    div.textContent = textBefore;
    const span = document.createElement('span');
    span.textContent = element.value.substring(position, position + 1) || '.';
    div.appendChild(span);

    document.body.appendChild(div);
    const targetTop = span.offsetTop;
    document.body.removeChild(div);

    return targetTop;
  }

  function scrollToPosition(element: HTMLTextAreaElement, position: number, matchLength: number) {
    if (!element || position < 0) return;

    element.focus();
    element.setSelectionRange(position, position + matchLength);

    const applyScroll = () => {
      const targetTop = getScrollPositionOfIndex(element, position);
      const centeredScrollTop = Math.max(0, targetTop - element.clientHeight / 2);
      element.scrollTop = centeredScrollTop;
    };

    applyScroll();
    requestAnimationFrame(applyScroll);
    setTimeout(applyScroll, 20);
  }

  function handleCustomSearch() {
    if (!findQuery.trim()) {
      toast.show('Please enter a search keyword or Regex pattern!', 'error');
      return;
    }
    if (!textareaElement) return;

    let targetIndex = -1;
    let matchLength = 0;

    if (isRegexMode) {
      try {
        const regex = new RegExp(findQuery, 'gi');
        let match;
        const matches = [];
        while ((match = regex.exec(text)) !== null) {
          matches.push(match);
        }

        if (matches.length === 0) {
          toast.show('No matches found for Regex pattern!', 'error');
          return;
        }

        let nextMatch = matches.find((m) => m.index > lastSearchIndex);
        if (!nextMatch) {
          nextMatch = matches[0];
        }

        targetIndex = nextMatch.index;
        matchLength = nextMatch[0].length;
        lastSearchIndex = targetIndex + matchLength;
      } catch (err) {
        toast.show('Invalid Regex expression!', 'error');
        return;
      }
    } else {
      const lowerText = text.toLowerCase();
      const lowerQuery = findQuery.toLowerCase();
      let foundPos = lowerText.indexOf(lowerQuery, lastSearchIndex);

      if (foundPos === -1 && lastSearchIndex > 0) {
        foundPos = lowerText.indexOf(lowerQuery, 0);
      }

      if (foundPos !== -1) {
        targetIndex = foundPos;
        matchLength = findQuery.length;
        lastSearchIndex = foundPos + matchLength;
      } else {
        toast.show('Keyword not found in text!', 'error');
        return;
      }
    }

    if (targetIndex !== -1) {
      scrollToPosition(textareaElement, targetIndex, matchLength);
      toast.show('Found match! (Click again for next match)', 'success');
    }
  }

  function handleCustomReplace() {
    if (isReadOnly) {
      toast.show('Text loaded from history is in read-only mode!', 'error');
      return;
    }
    if (!findQuery.trim()) {
      toast.show('Please enter keyword to replace!', 'error');
      return;
    }

    let oldText = text;
    let newText = text;

    if (isRegexMode) {
      try {
        const regex = new RegExp(findQuery, 'gi');
        newText = text.replace(regex, replaceQuery);
      } catch (err) {
        toast.show('Invalid Regex expression!', 'error');
        return;
      }
    } else {
      const regex = new RegExp(findQuery.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'gi');
      newText = text.replace(regex, replaceQuery);
    }

    if (oldText !== newText) {
      text = newText;
      toast.show('Replaced occurrences successfully!', 'success');
      lastSearchIndex = 0;
    } else {
      toast.show('String to replace not found!', 'info');
    }
  }

  function findChunkIndex(fullText: string, chunk: string): number {
    let exactIdx = fullText.indexOf(chunk);
    if (exactIdx !== -1) return exactIdx;

    let startStr = chunk.substring(0, 40).replace(/\s+/g, '');
    if (!startStr) return -1;

    let regexStr = startStr.split('').map((c) => c.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('\\s*');
    let match = fullText.match(new RegExp(regexStr));
    return match && match.index !== undefined ? match.index : -1;
  }

  function handleChunkClick(chunk: string, index: number) {
    if (!textareaElement) return;
    const idx = findChunkIndex(text, chunk);
    if (idx !== -1) {
      scrollToPosition(textareaElement, idx, chunk.length);
      toast.show(`Located Chunk ${index + 1}!`, 'success');
    } else {
      toast.show(`Could not locate Chunk ${index + 1}!`, 'error');
    }
  }
</script>

<div class="glass-panel" class:collapsed={isCollapsed} style="margin-bottom: 1.5rem; transition: all 0.3s ease;">
  <div class="form-group" style="margin-bottom: 0;">
    <div style="display: flex; justify-content: space-between; align-items: center;">
      <label for="main-text" style="margin-bottom: 0; font-size: 1.1rem; color: var(--primary); display: flex; align-items: center; gap: 8px;">
        <i class="fa-solid fa-file-lines"></i> Input Text
        {#if isReadOnly}
          <span class="badge" style="background: rgba(239, 68, 68, 0.2); color: #f87171; border: 1px solid rgba(239, 68, 68, 0.3); font-size: 0.75rem; margin-left: 8px;">
            <i class="fa-solid fa-lock"></i> Read-Only (History)
          </span>
        {/if}
      </label>
      <div style="display: flex; align-items: center; gap: 8px;">
        {#if inputPanelSpec === null || inputPanelSpec.file_serve}
          <div style="display: flex; align-items: center; gap: 8px;">
            {#if uploadedFileName}
              <span style="font-size: 0.8rem; color: var(--success); font-style: italic; max-width: 150px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">
                <i class="fa-solid fa-check"></i> {uploadedFileName}
              </span>
            {/if}
            <button class="upload-link" onclick={() => fileInput?.click()} style="background: none; border: none;">
              <i class="fa-solid fa-file-import"></i> Upload file
            </button>
            <button onclick={() => runAutoFormat(true)} disabled={isReadOnly} style="background: rgba(16, 185, 129, 0.15); color: #10b981; border: 1px solid rgba(16, 185, 129, 0.3); padding: 3px 10px; border-radius: 6px; font-size: 0.8em; font-weight: 500; cursor: {isReadOnly ? 'not-allowed' : 'pointer'}; opacity: {isReadOnly ? 0.5 : 1}; display: flex; align-items: center; gap: 4px;">
              <i class="fa-solid fa-wand-magic-sparkles"></i> Clean Text
            </button>
            <input id="document-upload-input" aria-label="Upload document file" type="file" bind:this={fileInput} onchange={handleFileUpload} accept=".txt,.pdf,.docx,.odt" class="hidden" />
          </div>
        {/if}
        <button
          onclick={() => isCollapsed = !isCollapsed}
          style="background: rgba(255,255,255,0.1); border: none; color: white; padding: 4px 10px; border-radius: 6px; font-size: 0.8em; display: flex; align-items: center; gap: 5px; cursor: pointer;"
        >
          <i class="fa-solid {isCollapsed ? 'fa-chevron-down' : 'fa-chevron-up'}"></i>
          <span>{isCollapsed ? 'Expand' : 'Collapse'}</span>
        </button>
      </div>
    </div>

    {#if !isCollapsed}
      <div style="margin-top: 0.5rem">
        <textarea
          id="main-text"
          bind:this={textareaElement}
          rows="8"
          spellcheck="false"
          bind:value={text}
          readonly={isReadOnly}
          onblur={() => runAutoFormat(false)}
          placeholder="Enter or paste your text here..."
          style="opacity: {isReadOnly ? 0.7 : 1}; cursor: {isReadOnly ? 'not-allowed' : 'text'};"
        ></textarea>

        <!-- Search and Replace Tools -->
        {#if inputPanelSpec === null || inputPanelSpec.replace_tool}
          <div class="search-replace-container">
            <div class="search-replace-inputs">
              <!-- Row 1: Search -->
              <div class="search-input-group">
                {#if inputPanelSpec === null || inputPanelSpec.find_mode === 'expert'}
                  <button
                    onmousedown={(e) => e.preventDefault()}
                    onclick={toggleRegexMode}
                    class="regex-toggle-btn desktop-only-btn"
                    style="color: {isRegexMode ? 'var(--primary)' : 'var(--text-muted)'};"
                  >
                    <i class="fa-solid {isRegexMode ? 'fa-code' : 'fa-font'}"></i> {isRegexMode ? 'Regex' : 'Basic'}
                  </button>
                {/if}
                <input
                  type="text"
                  bind:value={findQuery}
                  placeholder={isRegexMode ? 'Regex pattern (e.g. \\[\\d+\\])' : 'Find text...'}
                  class="search-input"
                />
                <button
                  onmousedown={(e) => e.preventDefault()}
                  onclick={handleCustomSearch}
                  class="search-btn desktop-only-btn"
                >
                  <i class="fa-solid fa-magnifying-glass"></i> Find
                </button>
              </div>

              <!-- Row 2: Replace -->
              <div class="replace-input-group" style="margin-top: 8px;">
                <div class="replace-arrow-icon"><i class="fa-solid fa-arrow-down"></i></div>
                <input
                  type="text"
                  bind:value={replaceQuery}
                  readonly={isReadOnly}
                  placeholder={isReadOnly ? 'Locked in history view' : 'Replace with (Leave blank to remove)'}
                  class="replace-input"
                  style="opacity: {isReadOnly ? 0.7 : 1}; cursor: {isReadOnly ? 'not-allowed' : 'text'};"
                />
                <button
                  onmousedown={(e) => e.preventDefault()}
                  onclick={handleCustomReplace}
                  disabled={isReadOnly}
                  class="replace-btn desktop-only-btn"
                  style="opacity: {isReadOnly ? 0.5 : 1}; cursor: {isReadOnly ? 'not-allowed' : 'pointer'};"
                >
                  <i class="fa-solid fa-check"></i> Replace
                </button>
              </div>
            </div>

            <!-- Mobile Action Buttons Row (3 buttons in 1 row under inputs) -->
            <div class="mobile-search-actions-row">
              {#if inputPanelSpec === null || inputPanelSpec.find_mode === 'expert'}
                <button
                  onmousedown={(e) => e.preventDefault()}
                  onclick={toggleRegexMode}
                  class="regex-toggle-btn"
                  style="color: {isRegexMode ? 'var(--primary)' : 'var(--text-muted)'};"
                >
                  <i class="fa-solid {isRegexMode ? 'fa-code' : 'fa-font'}"></i> {isRegexMode ? 'Regex' : 'Basic'}
                </button>
              {/if}
              <button
                onmousedown={(e) => e.preventDefault()}
                onclick={handleCustomSearch}
                class="search-btn"
              >
                <i class="fa-solid fa-magnifying-glass"></i> Find
              </button>
              <button
                onmousedown={(e) => e.preventDefault()}
                onclick={handleCustomReplace}
                disabled={isReadOnly}
                class="replace-btn"
                style="opacity: {isReadOnly ? 0.5 : 1}; cursor: {isReadOnly ? 'not-allowed' : 'pointer'};"
              >
                <i class="fa-solid fa-check"></i> Replace
              </button>
            </div>
          </div>
        {/if}

        <!-- Visual Chunks Indicator -->
        {#if chunks.length > 1 && (inputPanelSpec === null || inputPanelSpec.enable_chunk_box)}
          <div style="margin-top: 15px; background: rgba(0,0,0,0.15); padding: 15px; border-radius: 10px; border: 1px solid rgba(255,255,255,0.05);">
            <div style="font-size: 0.9em; color: var(--text-muted); margin-bottom: 12px; display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 8px;">
              <span><i class="fa-solid fa-layer-group"></i> Text split into <strong style="color: var(--primary); font-size: 1.1em;">{chunks.length}</strong> chunks (click to locate):</span>
              <span style="font-size: 0.85em; opacity: 0.7;"><i class="fa-solid fa-scissors"></i> Max {maxChunkSize.toLocaleString()} chars/chunk</span>
            </div>
            <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 10px; max-height: 140px; overflow-y: auto; padding-right: 5px;">
              {#each chunks as chunk, i}
                <button
                  type="button"
                  onmousedown={(e) => e.preventDefault()}
                  onclick={() => handleChunkClick(chunk, i)}
                  style="background: rgba(255,255,255,0.06); padding: 8px 10px; border-radius: 6px; font-size: 0.85em; border: 1px solid rgba(255,255,255,0.1); color: white; text-align: left; cursor: pointer; transition: all 0.2s; display: block; width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;"
                  title="Click to locate Chunk {i + 1} in text"
                >
                  <strong style="color: var(--primary);">Chunk {i + 1}:</strong> {chunk.substring(0, 50).replace(/\n/g, ' ')}... ({chunk.length} chars)
                </button>
              {/each}
            </div>
          </div>
        {/if}
      </div>
    {/if}
  </div>
</div>
