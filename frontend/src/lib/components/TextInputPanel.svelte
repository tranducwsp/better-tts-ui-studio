<script lang="ts">
  import type { InputPanelSpec } from '../types';
  import { extractTextFromFile } from '../api';
  import { toast } from '../toast.svelte';

  interface Props {
    text: string;
    isReadOnly?: boolean;
    inputPanelSpec?: InputPanelSpec | null;
  }

  let { text = $bindable(), isReadOnly = $bindable(false), inputPanelSpec = null }: Props = $props();

  let isCollapsed = $state(false);
  let isRegexMode = $state(false);
  let findQuery = $state('');
  let replaceQuery = $state('');
  let uploadedFileName = $state('');
  let lastSearchIndex = $state(0);
  let textareaElement: HTMLTextAreaElement;

  // Svelte 5 derived state for text chunks (Exact match with original splitTextIntoChunks)
  let chunks = $derived.by(() => {
    if (!text || text.length <= 1000) return text ? [text] : [];
    const paragraphs = text.split(/(?<=\.\s*\n)/);
    const result: string[] = [];
    let currentChunk = '';

    for (const p of paragraphs) {
      const cleanP = p.trim();
      if (!cleanP) continue;

      if (cleanP.length > 2000) {
        const sentences = cleanP.match(/[^.!?]+[.!?]+/g) || [cleanP];
        for (const s of sentences) {
          const cleanS = s.trim();
          if (!cleanS) continue;
          if (currentChunk.length + cleanS.length + 1 <= 1000) {
            currentChunk += (currentChunk ? ' ' : '') + cleanS;
          } else {
            if (currentChunk) result.push(currentChunk);
            currentChunk = cleanS;
          }
        }
      } else {
        if (currentChunk.length + cleanP.length + 1 <= 1000) {
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

  let fileInput: HTMLInputElement;

  async function handleFileUpload(e: Event) {
    const target = e.target as HTMLInputElement;
    if (!target.files?.length) return;
    const file = target.files[0];
    toast.show('Đang đọc tài liệu...', 'info');
    try {
      const extracted = await extractTextFromFile(file);
      if (extracted) {
        text = extracted;
        isReadOnly = false;
        uploadedFileName = file.name;
        toast.show('Đã tải xong văn bản!', 'success');
      }
    } catch (err: any) {
      toast.show('Lỗi đọc file: ' + err.message, 'error');
    }
    target.value = ''; // Reset input
  }

  function toggleRegexMode() {
    isRegexMode = !isRegexMode;
    lastSearchIndex = 0;
  }

  function getScrollPositionOfIndex(element: HTMLTextAreaElement, position: number): number {
    const textBefore = element.value.substring(0, position);
    const linesBefore = textBefore.split('\n').length - 1;
    const computedStyle = window.getComputedStyle(element);
    let lineHeight = parseFloat(computedStyle.lineHeight);
    if (isNaN(lineHeight)) {
      const fontSize = parseFloat(computedStyle.fontSize);
      lineHeight = fontSize * 1.5;
    }
    return linesBefore * lineHeight;
  }

  function handleCustomSearch() {
    if (!findQuery.trim()) {
      toast.show('Vui lòng nhập từ khóa hoặc mẫu Regex cần tìm!', 'error');
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
          toast.show('Không tìm thấy kết quả phù hợp với Regex!', 'error');
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
        toast.show('Cú pháp Regex không hợp lệ!', 'error');
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
        toast.show('Không tìm thấy từ khóa trong văn bản!', 'error');
        return;
      }
    }

    if (targetIndex !== -1) {
      const targetTop = getScrollPositionOfIndex(textareaElement, targetIndex);
      textareaElement.scrollTop = targetTop - textareaElement.clientHeight / 2;
      textareaElement.setSelectionRange(targetIndex, targetIndex + matchLength);
      textareaElement.focus();
      toast.show('Đã tìm thấy (Bấm tiếp để tìm kết quả tiếp theo)', 'success');
    }
  }

  function handleCustomReplace() {
    if (isReadOnly) {
      toast.show('Văn bản nạp từ lịch sử đang ở chế độ chỉ đọc!', 'error');
      return;
    }
    if (!findQuery.trim()) {
      toast.show('Vui lòng nhập từ khóa cần thay thế!', 'error');
      return;
    }

    let oldText = text;
    let newText = text;

    if (isRegexMode) {
      try {
        const regex = new RegExp(findQuery, 'gi');
        newText = text.replace(regex, replaceQuery);
      } catch (err) {
        toast.show('Cú pháp Regex không hợp lệ!', 'error');
        return;
      }
    } else {
      const regex = new RegExp(findQuery.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'gi');
      newText = text.replace(regex, replaceQuery);
    }

    if (oldText !== newText) {
      text = newText;
      toast.show('Đã thay thế hàng loạt thành công!', 'success');
      lastSearchIndex = 0;
    } else {
      toast.show('Không tìm thấy chuỗi cần thay thế!', 'info');
    }
  }

  function findChunkIndex(fullText: string, chunk: string): number {
    let exactIdx = fullText.indexOf(chunk);
    if (exactIdx !== -1) return exactIdx;

    let startStr = chunk.substring(0, 40).replace(/\s+/g, '');
    if (!startStr) return -1;

    let regexStr = startStr.split('').map((c) => c.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('\\s*');
    let match = fullText.match(new RegExp(regexStr));
    return match ? match.index : -1;
  }

  function handleChunkClick(chunk: string, index: number) {
    if (!textareaElement) return;
    const idx = findChunkIndex(text, chunk);
    if (idx !== -1) {
      const targetTop = getScrollPositionOfIndex(textareaElement, idx);
      textareaElement.scrollTop = targetTop - textareaElement.clientHeight / 2;
      textareaElement.setSelectionRange(idx, idx + chunk.length);
      textareaElement.focus();
      toast.show(`Đã định vị Đoạn ${index + 1}!`, 'success');
    } else {
      toast.show(`Không tìm thấy vị trí Đoạn ${index + 1}`, 'error');
    }
  }
</script>

<div class="glass-panel" class:collapsed={isCollapsed} style="margin-bottom: 1.5rem; transition: all 0.3s ease;">
  <div class="form-group" style="margin-bottom: 0;">
    <div style="display: flex; justify-content: space-between; align-items: center;">
      <label for="main-text" style="margin-bottom: 0; font-size: 1.1rem; color: var(--primary); display: flex; align-items: center; gap: 8px;">
        <i class="fa-solid fa-file-lines"></i> Nội dung văn bản
        {#if isReadOnly}
          <span class="badge" style="background: rgba(239, 68, 68, 0.2); color: #f87171; border: 1px solid rgba(239, 68, 68, 0.3); font-size: 0.75rem; margin-left: 8px;">
            <i class="fa-solid fa-lock"></i> Chỉ đọc (Lịch sử)
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
              <i class="fa-solid fa-file-import"></i> Tải file lên
            </button>
            <input type="file" bind:this={fileInput} onchange={handleFileUpload} accept=".txt,.pdf,.docx,.odt" class="hidden" />
          </div>
        {/if}
        <button
          onclick={() => isCollapsed = !isCollapsed}
          style="background: rgba(255,255,255,0.1); border: none; color: white; padding: 4px 10px; border-radius: 6px; font-size: 0.8em; display: flex; align-items: center; gap: 5px; cursor: pointer;"
        >
          <i class="fa-solid {isCollapsed ? 'fa-chevron-down' : 'fa-chevron-up'}"></i>
          <span>{isCollapsed ? 'Mở rộng' : 'Thu gọn'}</span>
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
          placeholder="Nhập văn bản tiếng Việt của bạn vào đây..."
          style="opacity: {isReadOnly ? 0.7 : 1}; cursor: {isReadOnly ? 'not-allowed' : 'text'};"
        ></textarea>

        <!-- Tool thay thế rác / tìm kiếm -->
        {#if inputPanelSpec === null || inputPanelSpec.replace_tool}
          <div style="display: flex; flex-direction: column; gap: 8px; margin-top: 10px; background: rgba(0,0,0,0.2); padding: 10px 12px; border-radius: 8px; border: 1px solid rgba(255,255,255,0.05);">
            <!-- Hàng 1: Dò tìm (Luôn mở) -->
            <div style="display: flex; gap: 10px; align-items: center;">
              {#if inputPanelSpec === null || inputPanelSpec.find_mode === 'expert'}
                <button
                  onmousedown={(e) => e.preventDefault()}
                  onclick={toggleRegexMode}
                  style="background: rgba(255,255,255,0.1); color: {isRegexMode ? 'var(--primary)' : 'var(--text-muted)'}; border: none; padding: 6px 10px; border-radius: 6px; font-weight: 500; cursor: pointer; font-size: 0.8em; min-width: 85px;"
                >
                  <i class="fa-solid {isRegexMode ? 'fa-code' : 'fa-font'}"></i> {isRegexMode ? 'Regex' : 'Cơ bản'}
                </button>
              {/if}
              <input
                type="text"
                bind:value={findQuery}
                placeholder={isRegexMode ? 'Nhập Regex (VD: \\[\d+\\])' : 'Tìm rác (VD: hhhggg)'}
                style="flex: 1; padding: 6px 10px; font-size: 0.9em; background: rgba(255,255,255,0.05); border: 1px solid rgba(255,255,255,0.1); border-radius: 6px; color: white;"
              />
              <button
                onmousedown={(e) => e.preventDefault()}
                onclick={handleCustomSearch}
                style="background: rgba(255,255,255,0.15); color: white; border: none; padding: 6px 12px; border-radius: 6px; font-weight: 500; cursor: pointer; display: flex; align-items: center; gap: 5px; font-size: 0.9em; white-space: nowrap;"
              >
                <i class="fa-solid fa-magnifying-glass"></i> Dò tìm
              </button>
            </div>

            <!-- Hàng 2: Thay thế (Khóa khi isReadOnly = true) -->
            <div style="display: flex; gap: 10px; align-items: center;">
              <div style="min-width: 85px; text-align: center; color: var(--text-muted);"><i class="fa-solid fa-arrow-down"></i></div>
              <input
                type="text"
                bind:value={replaceQuery}
                readonly={isReadOnly}
                placeholder={isReadOnly ? 'Đã khóa khi xem Lịch sử' : 'Sửa thành (Để trống = Xóa)'}
                style="flex: 1; padding: 6px 10px; font-size: 0.9em; background: rgba(255,255,255,0.05); border: 1px solid rgba(255,255,255,0.1); border-radius: 6px; color: white; opacity: {isReadOnly ? 0.7 : 1}; cursor: {isReadOnly ? 'not-allowed' : 'text'};"
              />
              <button
                onmousedown={(e) => e.preventDefault()}
                onclick={handleCustomReplace}
                disabled={isReadOnly}
                style="background: var(--primary); color: white; border: none; padding: 6px 12px; border-radius: 6px; font-weight: 500; cursor: {isReadOnly ? 'not-allowed' : 'pointer'}; opacity: {isReadOnly ? 0.5 : 1}; font-size: 0.9em; min-width: 88px;"
              >
                <i class="fa-solid fa-check"></i> Thay thế
              </button>
            </div>
          </div>
        {/if}

        <!-- Visual Chunks Indicator -->
        {#if chunks.length > 1 && (inputPanelSpec === null || inputPanelSpec.enable_chunk_box)}
          <div style="margin-top: 15px; background: rgba(0,0,0,0.15); padding: 15px; border-radius: 10px; border: 1px solid rgba(255,255,255,0.05);">
            <div style="font-size: 0.9em; color: var(--text-muted); margin-bottom: 12px;">
              <span><i class="fa-solid fa-layer-group"></i> Văn bản đã chia nhỏ thành <strong style="color: var(--primary); font-size: 1.1em;">{chunks.length}</strong> đoạn (bấm để định vị):</span>
            </div>
            <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 10px; max-height: 140px; overflow-y: auto; padding-right: 5px;">
              {#each chunks as chunk, i}
                <button
                  type="button"
                  onmousedown={(e) => e.preventDefault()}
                  onclick={() => handleChunkClick(chunk, i)}
                  style="background: rgba(255,255,255,0.06); padding: 8px 10px; border-radius: 6px; font-size: 0.85em; border: 1px solid rgba(255,255,255,0.1); color: white; text-align: left; cursor: pointer; transition: all 0.2s; display: block; width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;"
                  title="Nhấn để định vị Đoạn {i + 1} trong văn bản"
                >
                  <strong style="color: var(--primary);">Đoạn {i + 1}:</strong> {chunk.substring(0, 50).replace(/\n/g, ' ')}... ({chunk.length} ký tự)
                </button>
              {/each}
            </div>
          </div>
        {/if}
      </div>
    {/if}
  </div>
</div>
