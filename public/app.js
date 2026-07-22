let currentJobId = "";
document.addEventListener('DOMContentLoaded', () => {
    // ---- DOM Elements ----
    const tabs = document.querySelectorAll('.tab-btn');
    const tabContents = document.querySelectorAll('.tab-content');
    
    // Main Text Input
    const mainText = document.getElementById('main-text');
    
    // Standard TTS
    const stdVoice = document.getElementById('std-voice');
    const customVoiceSelect = document.getElementById('custom-voice-select');
    const customVoiceOptions = document.getElementById('custom-voice-options');
    const selectedVoiceInfo = document.getElementById('selected-voice-info');

    const stdSpeed = document.getElementById('std-speed');
    const stdSpeedVal = document.getElementById('std-speed-val');
    const btnSynthesize = document.getElementById('btn-synthesize');
    const stdLoading = document.getElementById('std-loading');
    const stdResult = document.getElementById('std-result');
    const stdAudio = document.getElementById('std-audio');
    
    // Clone TTS
    const cloneFile = document.getElementById('clone-file');
    const uploadArea = document.getElementById('upload-area');
    const uploadAreaText = document.getElementById('upload-area-text');
    
    // Audio Trimmer
    const trimmerDiv = document.getElementById('audio-trimmer');
    const trimStartVal = document.getElementById('trim-start-val');
    const trimEndVal = document.getElementById('trim-end-val');
    const trimDurationVal = document.getElementById('trim-duration-val');
    const waveformCanvas = document.getElementById('waveform-canvas');
    const trimPreviewAudio = document.getElementById('trim-preview-audio');
    const btnTrimOnly = document.getElementById('btn-trim-only');
    const btnTrimUpload = document.getElementById('btn-trim-upload');
    
    const cloneStatus = document.getElementById('clone-status');
    const cloneIdDisplay = document.getElementById('clone-id-display');
    const cloneGenerateSection = document.getElementById('clone-generate-section');
    // Clone TTS Elements
    const cloneSpeed = document.getElementById('clone-speed');
    const cloneSpeedVal = document.getElementById('clone-speed-val');
    const btnSynthClone = document.getElementById('btn-synth-clone');
    const cloneLoading = document.getElementById('clone-loading');
    const cloneResult = document.getElementById('clone-result');
    const cloneAudio = document.getElementById('clone-audio');

    let currentCloneId = null;

    // ---- Initialize ----
    fetchVoices();

    // ---- Event Listeners ----
    
    // Tabs
    tabs.forEach(tab => {
        tab.addEventListener('click', () => {
            tabs.forEach(t => t.classList.remove('active'));
            tabContents.forEach(c => c.classList.remove('active'));
            
            tab.classList.add('active');
            document.getElementById(`${tab.dataset.tab}-tab`).classList.add('active');
        });
    });

    // Custom Select Dropdown logic
    if (customVoiceSelect) {
        customVoiceSelect.querySelector('.custom-select-trigger').addEventListener('click', () => {
            customVoiceSelect.classList.toggle('open');
        });

        // Close when click outside
        document.addEventListener('click', (e) => {
            if (!customVoiceSelect.contains(e.target)) {
                customVoiceSelect.classList.remove('open');
            }
        });
    }

    // Speed Sliders
    stdSpeed.addEventListener('input', (e) => {
        stdSpeedVal.textContent = `${parseFloat(e.target.value).toFixed(1)}x`;
    });
    
    cloneSpeed.addEventListener('input', (e) => {
        cloneSpeedVal.textContent = `${parseFloat(e.target.value).toFixed(1)}x`;
    });

    // Warning Modal Logic
    const warningModal = document.getElementById('warning-modal');
    const btnModalCancel = document.getElementById('btn-modal-cancel');
    const btnModalConfirm = document.getElementById('btn-modal-confirm');
    let pendingSynthesisTask = null;

    if (warningModal && btnModalCancel && btnModalConfirm) {
        btnModalCancel.addEventListener('click', () => {
            warningModal.classList.add('hidden');
            pendingSynthesisTask = null;
        });
        btnModalConfirm.addEventListener('click', async () => {
            warningModal.classList.add('hidden');
            if (pendingSynthesisTask) {
                const task = pendingSynthesisTask;
                pendingSynthesisTask = null;
                await task();
            }
        });
    }

    function executeWithWarning(taskCallback) {
        if (warningModal) {
            pendingSynthesisTask = taskCallback;
            warningModal.classList.remove('hidden');
        } else {
            taskCallback();
        }
    }




    // Document Upload Handling
    const btnDocMain = document.getElementById('btn-upload-doc-main');
    const fileDocMain = document.getElementById('file-doc-main');
    const filenameSpanMain = document.getElementById('doc-filename-main');

    if (btnDocMain) btnDocMain.addEventListener('click', () => fileDocMain.click());

    async function handleDocUpload(e, targetTextarea, targetFilenameSpan) {
        if (!e.target.files.length) return;
        const file = e.target.files[0];
        
        const formData = new FormData();
        formData.append('file', file);
        
        // Các tham số lọc trang đã được bỏ theo yêu cầu
        
        showToast('Đang đọc tài liệu...', 'success');
        
        try {
            const response = await fetch('/api/extract-text', {
                method: 'POST',
                body: formData
            });

            const data = await response.json();
            if (!response.ok) throw new Error(data.detail || 'Lỗi server');

            targetTextarea.value = data.text;
            if (targetTextarea === mainText) {
                processMainTextChunks();
            }
            if (targetFilenameSpan) {
                targetFilenameSpan.innerHTML = `<i class="fa-solid fa-check"></i> ${file.name}`;
                targetFilenameSpan.title = file.name;
            }
            showToast('Đã tải xong văn bản!', 'success');
        } catch (error) {
            showToast('Lỗi đọc file: ' + error.message, 'error');
        }
        
        e.target.value = ''; // Reset input
    }

    if (fileDocMain) fileDocMain.addEventListener('change', (e) => handleDocUpload(e, mainText, filenameSpanMain));

    // ---- CHUNKING LOGIC ----
    const chunkListContainer = document.getElementById('chunk-list-container');
    const chunkList = document.getElementById('chunk-list');
    const chunkCountSpan = document.getElementById('chunk-count');

    function splitTextIntoChunks(text, targetChars = 1000, maxChars = 2000) {
        if (text.length <= targetChars) return [text];
        
        // Bước 3: Tách câu ưu tiên bằng dấu chấm + xuống dòng (.\n)
        const paragraphs = text.split(/(?<=\.\s*\n)/);
        
        const chunks = [];
        let currentChunk = "";
        
        for (let p of paragraphs) {
            const cleanP = p.trim();
            if (!cleanP) continue;
            
            // Bước 4: Fallback - Nếu đoạn này dài hơn 2000 ký tự (maxChars), buộc phải băm nhỏ tiếp bằng dấu . ? !
            if (cleanP.length > maxChars) {
                const sentences = cleanP.match(/[^.!?]+[.!?]+/g) || [cleanP];
                for (let s of sentences) {
                    const cleanS = s.trim();
                    if (!cleanS) continue;
                    if (currentChunk.length + cleanS.length + 1 <= targetChars) {
                        currentChunk += (currentChunk ? " " : "") + cleanS;
                    } else {
                        if (currentChunk) chunks.push(currentChunk);
                        currentChunk = cleanS;
                    }
                }
            } else {
                // Gom bình thường nếu đoạn dưới 2000 ký tự
                if (currentChunk.length + cleanP.length + 1 <= targetChars) {
                    currentChunk += (currentChunk ? "\n" : "") + cleanP;
                } else {
                    if (currentChunk) chunks.push(currentChunk);
                    currentChunk = cleanP;
                }
            }
        }
        if (currentChunk) chunks.push(currentChunk);
        return chunks.length ? chunks : [text];
    }

    // Logic cho công cụ sửa lỗi tùy chỉnh (Tìm và Thay thế)
    const customFind = document.getElementById('custom-find');
    const customReplace = document.getElementById('custom-replace');
    const btnCustomReplace = document.getElementById('btn-custom-replace');
    const btnRegexMode = document.getElementById('btn-regex-mode');
    const btnCustomSearch = document.getElementById('btn-custom-search');
    
    let isRegexMode = false;
    let lastSearchIndex = 0;

    if (btnRegexMode) {
        btnRegexMode.addEventListener('click', () => {
            isRegexMode = !isRegexMode;
            if (isRegexMode) {
                btnRegexMode.innerHTML = '<i class="fa-solid fa-code"></i> Regex';
                btnRegexMode.style.color = 'var(--primary)';
                customFind.placeholder = 'Nhập Regex (VD: \\[\d+\\])';
            } else {
                btnRegexMode.innerHTML = '<i class="fa-solid fa-font"></i> Cơ bản';
                btnRegexMode.style.color = 'var(--text-muted)';
                customFind.placeholder = 'Tìm rác (VD: hhhggg)';
            }
            lastSearchIndex = 0; // reset
            customFind.focus();
        });
        // Prevent textarea blur when clicking regex switch
        btnRegexMode.onmousedown = (e) => e.preventDefault();
    }

    if (btnCustomSearch) {
        btnCustomSearch.addEventListener('click', () => {
            const findStr = customFind.value;
            if (!findStr) {
                showToast('Vui lòng nhập chuỗi cần dò!', 'error');
                return;
            }
            
            const text = mainText.value;
            let matchIdx = -1;
            let matchLen = 0;
            
            if (isRegexMode) {
                try {
                    const regex = new RegExp(findStr, 'g');
                    regex.lastIndex = lastSearchIndex;
                    let match = regex.exec(text);
                    if (!match) {
                        regex.lastIndex = 0; // Quay vòng lại đầu
                        match = regex.exec(text);
                    }
                    if (match) {
                        matchIdx = match.index;
                        matchLen = match[0].length;
                        lastSearchIndex = regex.lastIndex;
                    }
                } catch (e) {
                    showToast('Cú pháp Regex không hợp lệ!', 'error');
                    return;
                }
            } else {
                matchIdx = text.indexOf(findStr, lastSearchIndex);
                if (matchIdx !== -1) {
                    matchLen = findStr.length;
                    lastSearchIndex = matchIdx + matchLen;
                } else {
                    matchIdx = text.indexOf(findStr); // Quay vòng lại đầu
                    if (matchIdx !== -1) {
                        matchLen = findStr.length;
                        lastSearchIndex = matchIdx + matchLen;
                    }
                }
            }
            
            if (matchIdx !== -1) {
                // Tính toán vị trí cuộn cực kỳ chính xác bằng shadow div
                const targetTop = getScrollPositionOfIndex(mainText, matchIdx);
                // Cuộn sao cho kết quả nằm ở giữa màn hình (trừ đi một nửa chiều cao)
                mainText.scrollTop = targetTop - (mainText.clientHeight / 2);
                
                mainText.setSelectionRange(matchIdx, matchIdx + matchLen);
                mainText.focus();
                
                showToast('Đã tìm thấy (Bấm tiếp để tìm kết quả tiếp theo)', 'success');
            } else {
                showToast('Không tìm thấy kết quả nào!', 'info');
                lastSearchIndex = 0;
            }
        });
        // Prevent blur
        btnCustomSearch.onmousedown = (e) => e.preventDefault();
    }

    if (btnCustomReplace) {
        btnCustomReplace.addEventListener('click', () => {
            const findStr = customFind.value;
            const replaceStr = customReplace.value; // Có thể để rỗng để xóa
            
            if (!findStr) {
                showToast('Vui lòng nhập chuỗi cần tìm!', 'error');
                return;
            }
            
            const oldText = mainText.value;
            let newText = oldText;
            
            if (isRegexMode) {
                try {
                    const regex = new RegExp(findStr, 'g');
                    newText = oldText.replace(regex, replaceStr);
                } catch (e) {
                    showToast('Cú pháp Regex không hợp lệ!', 'error');
                    return;
                }
            } else {
                newText = oldText.split(findStr).join(replaceStr);
            }
            
            if (oldText !== newText) {
                mainText.value = newText;
                processMainTextChunks(); // Cập nhật lại các chunk bên dưới
                showToast(`Đã thay thế hàng loạt thành công!`, 'success');
                lastSearchIndex = 0;
            } else {
                showToast(`Không tìm thấy chuỗi cần thay thế!`, 'info');
            }
        });
        // Prevent blur
        btnCustomReplace.onmousedown = (e) => e.preventDefault();
    }

    function processMainTextChunks() {
        let text = mainText.value.trim();
        if (!text) {
            if (chunkListContainer) chunkListContainer.classList.add('hidden');
            return;
        }

        // Chuẩn hóa bộ gõ (\r\n thành \n) để thuật toán regex nhận diện đúng
        text = text.replace(/\r\n/g, '\n');
        // Bước 1: Giảm các khoảng trắng thừa dòng (nhiều hơn 2) về tối đa 2 \n để giữ lại khoảng cách đoạn
        text = text.replace(/\n{3,}/g, '\n\n');
        
        // Bước 2: Chuẩn hóa chữ Ð và Hán Việt
        text = text.replace(/\u00D0/g, '\u0110'); // \u00D0 là Ð (Eth), \u0110 là Đ (Việt Nam)
        text = text.replace(/([a-zA-ZÀ-ỹ])\-([a-zA-ZÀ-ỹ])/g, "$1 $2"); // Gỡ gạch nối
        
        // Bước 3: Xóa các chữ số Unicode dạng mũ/cước chú (Superscript/Subscript) như ¹, ², ³...
        text = text.replace(/[⁰¹²³⁴⁵⁶⁷⁸⁹₀₁₂₃₄₅₆₇₈₉]/g, '');
        
        // Bước 4: Loại bỏ toàn bộ ký tự rác (emoji, ký hiệu toán học lạ...), GIỮ LẠI ngoặc vuông []
        text = text.replace(/[^a-zA-Z0-9 \n\t\r.,?!;:"'()\[\]\-%/“”‘’À-ỹ]/g, '');
        
        text = text.trim();
        
        if (mainText.value !== text) {
            mainText.value = text;
        }

        const chunks = splitTextIntoChunks(text, 1000, 2000);
        if (chunks.length <= 1) {
            if (chunkListContainer) chunkListContainer.classList.add('hidden');
            return;
        }
        
        if (chunkListContainer && chunkList && chunkCountSpan) {
            chunkList.innerHTML = '';
            const fragment = document.createDocumentFragment();
            
            chunks.forEach((chunk, index) => {
                const btn = document.createElement('button');
                btn.className = 'btn secondary-btn';
                btn.style.cssText = 'display: block; padding: 8px 16px; font-size: 0.9em; width: 100%; text-align: left; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; border-radius: 6px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); transition: all 0.2s ease;';
                // Làm phẳng văn bản (xóa dấu xuống dòng) để hiển thị trên 1 dòng
                let previewText = chunk.substring(0, 40).replace(/\n/g, ' ').replace(/\s+/g, ' ').trim();
                btn.innerText = `${previewText}...`;
                btn.title = chunk; // Tooltip to read the whole text on hover
                
                // Add hover effect manually if needed, or rely on secondary-btn class hover
                btn.onmouseover = () => btn.style.transform = 'translateY(-1px)';
                btn.onmouseout = () => btn.style.transform = 'none';
                
                // Ngăn textarea bị mất focus (blur) khi bấm mousedown, tránh re-render list
                btn.onmousedown = (e) => e.preventDefault();
                
                // Helper để tìm vị trí chunk bỏ qua sự khác biệt về khoảng trắng
                function findChunkIndex(fullText, chunk) {
                    let exactIdx = fullText.indexOf(chunk);
                    if (exactIdx !== -1) return exactIdx;
                    
                    let startStr = chunk.substring(0, 40).replace(/\s+/g, '');
                    if (!startStr) return -1;
                    
                    // Tạo regex cho phép khoảng trắng ẩn xen giữa các ký tự
                    let regexStr = startStr.split('').map(c => c.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('\\s*');
                    let match = fullText.match(new RegExp(regexStr));
                    return match ? match.index : -1;
                }

                btn.onclick = () => {
                    const idx = findChunkIndex(mainText.value, chunk);
                    if (idx !== -1) {
                        const targetTop = getScrollPositionOfIndex(mainText, idx);
                        mainText.scrollTop = targetTop - (mainText.clientHeight / 2);
                        
                        mainText.setSelectionRange(idx, idx + chunk.length);
                        mainText.focus();
                        
                        showToast(`Đã định vị Đoạn ${index + 1}!`, 'success');
                    } else {
                        showToast(`Không tìm thấy vị trí Đoạn ${index + 1}`, 'error');
                    }
                };
                fragment.appendChild(btn);
            });
            
            chunkList.appendChild(fragment);
            chunkCountSpan.innerText = chunks.length;
            chunkListContainer.classList.remove('hidden');
        }
    }

    // Tách chunk khi người dùng rời khỏi khung nhập liệu (blur)
    if (mainText) {
        mainText.addEventListener('blur', processMainTextChunks);
    }

    // ---- Helper Functions ----

    function audioBufferToWav(buffer, startSec, endSec) {
        const numChannels = buffer.numberOfChannels;
        const sampleRate = buffer.sampleRate;
        const startOffset = Math.floor(startSec * sampleRate);
        const endOffset = Math.floor(endSec * sampleRate);
        const lengthInSamples = endOffset - startOffset;
        
        const interleaved = new Float32Array(lengthInSamples * numChannels);
        for (let channel = 0; channel < numChannels; channel++) {
            const channelData = buffer.getChannelData(channel);
            let offset = channel;
            for (let i = startOffset; i < endOffset; i++) {
                interleaved[offset] = channelData[i];
                offset += numChannels;
            }
        }
        
        const wavBuffer = new ArrayBuffer(44 + interleaved.length * 2);
        const view = new DataView(wavBuffer);
        
        const writeString = (view, offset, string) => {
            for (let i = 0; i < string.length; i++) {
                view.setUint8(offset + i, string.charCodeAt(i));
            }
        };
        
        writeString(view, 0, 'RIFF');
        view.setUint32(4, 36 + interleaved.length * 2, true);
        writeString(view, 8, 'WAVE');
        writeString(view, 12, 'fmt ');
        view.setUint32(16, 16, true);
        view.setUint16(20, 1, true);
        view.setUint16(22, numChannels, true);
        view.setUint32(24, sampleRate, true);
        view.setUint32(28, sampleRate * numChannels * 2, true);
        view.setUint16(32, numChannels * 2, true);
        view.setUint16(34, 16, true);
        writeString(view, 36, 'data');
        view.setUint32(40, interleaved.length * 2, true);
        
        let offset = 44;
        for (let i = 0; i < interleaved.length; i++) {
            let sample = Math.max(-1, Math.min(1, interleaved[i]));
            sample = sample < 0 ? sample * 0x8000 : sample * 0x7FFF;
            view.setInt16(offset, sample, true);
            offset += 2;
        }
        
        return new Blob([view], { type: 'audio/wav' });
    }

    async function fetchVoices() {
        try {
            const res = await fetch('/api/standard/voices');
            if (!res.ok) throw new Error('Không thể tải giọng');
            const voices = await res.json();
            
            if (voices && voices.length > 0) {
                stdVoice.innerHTML = '';
                customVoiceOptions.innerHTML = '';
                
                let isFirst = true;

                voices.forEach(v => {
                    const optionText = Array.isArray(v) ? v[0] : v;
                    const optionValue = Array.isArray(v) ? v[1] : v;
                    
                    // Add to hidden select
                    const option = document.createElement('option');
                    option.textContent = optionText;
                    option.value = optionValue;
                    stdVoice.appendChild(option);

                    // Add to custom dropdown
                    // "Minh Đức — Nam · Bắc · Phong cách tin tức"
                    let name = optionValue;
                    let gender = "", region = "", style = "";
                    
                    if (optionText.includes('—')) {
                        const parts = optionText.split('—').map(s => s.trim());
                        name = parts[0];
                        if (parts[1]) {
                            const tags = parts[1].split('·').map(s => s.trim());
                            gender = tags[0] || "";
                            region = tags[1] || "";
                            style = tags[2] || "";
                        }
                    }

                    const customOpt = document.createElement('div');
                    customOpt.className = 'custom-option';
                    customOpt.dataset.value = optionValue;
                    
                    let badgesHtml = '';
                    if (gender) badgesHtml += `<span class="badge ${gender.toLowerCase() === 'nữ' ? 'gender-nu' : 'gender-nam'}">${gender}</span>`;
                    if (region) badgesHtml += `<span class="badge region">${region}</span>`;
                    if (style) badgesHtml += `<span class="badge style">${style}</span>`;

                    customOpt.innerHTML = `
                        <div class="voice-title">${name}</div>
                        <div class="voice-badges">${badgesHtml}</div>
                    `;

                    // Handle Select Option
                    customOpt.addEventListener('click', () => {
                        stdVoice.value = optionValue;
                        selectedVoiceInfo.innerHTML = `<strong>${name}</strong> <div style="display:flex;gap:5px;margin-left:10px;">${badgesHtml}</div>`;
                        
                        document.querySelectorAll('.custom-option').forEach(el => el.classList.remove('selected'));
                        customOpt.classList.add('selected');
                        customVoiceSelect.classList.remove('open');
                    });

                    customVoiceOptions.appendChild(customOpt);

                    if (isFirst) {
                        customOpt.click(); // Select first by default
                        isFirst = false;
                    }
                });
            }
        } catch (err) {
            console.error('Fetch voices error:', err);
        }
    }

    function setLoading(loaderEl, resultEl, isLoading, isError = false) {
        if (isLoading) {
            loaderEl.classList.remove('hidden');
            resultEl.classList.add('hidden');
            loaderEl.innerHTML = `
                <div style="font-weight: 500; font-size: 1.1rem; color: #fff; text-align: center;"><i class="fa-solid fa-spinner fa-spin"></i> Khởi tạo AI...</div>
                <div class="progress-bar-container"><div class="progress-bar" style="width: 0%"></div></div>
                <div style="text-align: center; margin-top: 10px;">
                    <button onclick="window.currentCancelTask && window.currentCancelTask()" class="btn" style="padding: 0.4rem 1rem; font-size: 0.9rem; background-color: #ef4444; color: white;"><i class="fa-solid fa-xmark"></i> Hủy bỏ</button>
                </div>
            `;
        } else {
            if (!isError) {
                const progressBar = loaderEl.querySelector('.progress-bar');
                if (progressBar) progressBar.style.width = '100%';
                
                setTimeout(() => {
                    loaderEl.classList.add('hidden');
                    if (!isError) resultEl.classList.remove('hidden');
                }, 500);
            } else {
                loaderEl.classList.add('hidden');
            }
        }
    }

    async function pollTask(taskId, loaderEl, resultEl, audioEl) {
        return new Promise((resolve, reject) => {
            const evtSource = new EventSource(`/api/stream/tasks/${taskId}`);
            let isCancelled = false;
            
            window.currentCancelTask = async () => {
                if (isCancelled) return;
                isCancelled = true;
                evtSource.close();
                try {
                    await fetch(`/api/tasks/${taskId}/cancel`, { method: 'POST' });
                } catch (e) {}
                setLoading(loaderEl, resultEl, false, true);
                reject(new Error('Bạn đã hủy tiến trình.'));
            };

            evtSource.onmessage = async (event) => {
                if (isCancelled) return;
                try {
                    const data = JSON.parse(event.data);
                    
                    if (data.status === 'cancelled') {
                        evtSource.close();
                        setLoading(loaderEl, resultEl, false, true);
                        reject(new Error('Tiến trình đã bị hủy.'));
                    } else if (data.status === 'error') {
                        evtSource.close();
                        reject(new Error(data.error || 'Lỗi hệ thống AI'));
                    } else if (data.status === 'done') {
                        evtSource.close();
                        
                        loaderEl.innerHTML = `
                            <div style="font-weight: 500; font-size: 1.1rem; color: #10b981; text-align: center;"><i class="fa-solid fa-check"></i> Hoàn thành! Đang tải xuống...</div>
                            <div class="progress-bar-container"><div class="progress-bar" style="width: 100%"></div></div>
                        `;
                        
                        const wavUrl = `/api/tasks/${taskId}/audio?format=wav`;
                        const mp3Url = `/api/tasks/${taskId}/audio?format=mp3`;
                        
                        // Tải file về dưới dạng Blob để trình duyệt hỗ trợ kéo thanh thời gian (seeking) 100%
                        fetch(wavUrl)
                            .then(response => response.blob())
                            .then(blob => {
                                audioEl.src = URL.createObjectURL(blob);
                            })
                            .catch(error => {
                                console.error("Lỗi khi load audio blob:", error);
                                audioEl.src = wavUrl; // fallback nếu lỗi
                            });
                        
                        // Cập nhật nút tải xuống tương ứng
                        let prefix = 'std';
                        if (audioEl.id === 'clone-audio') prefix = 'clone';
                        if (audioEl.id === 'fasttts-audio') prefix = 'fasttts';
                        
                        const wavBtn = document.getElementById(`${prefix}-download-wav`);
                        const mp3Btn = document.getElementById(`${prefix}-download-mp3`);
                        
                        if (wavBtn) wavBtn.href = wavUrl;
                        if (mp3Btn) mp3Btn.href = mp3Url;
                        
                        setLoading(loaderEl, resultEl, false);
                        resolve();
                    } else {
                        const p = data.progress || 0;
                        loaderEl.innerHTML = `
                            <div style="font-weight: 500; font-size: 1.1rem; color: #fff; text-align: center;"><i class="fa-solid fa-microchip fa-fade"></i> AI đang tổng hợp: ${p}%</div>
                            <div class="progress-bar-container"><div class="progress-bar" style="width: ${p}%"></div></div>
                            <div style="text-align: center; margin-top: 10px;">
                                <button onclick="window.currentCancelTask()" class="btn" style="padding: 0.4rem 1rem; font-size: 0.9rem; background-color: #ef4444; color: white;"><i class="fa-solid fa-xmark"></i> Hủy tiến trình</button>
                            </div>
                        `;
                    }
                } catch (e) {
                    evtSource.close();
                    reject(e);
                }
            };
            
            evtSource.onerror = () => {
                evtSource.close();
                reject(new Error('Mất kết nối với máy chủ (SSE Error)'));
            };
        });
    }

    // Fast TTS (Review Phim) Logic
    const fastttsSpeed = document.getElementById('fasttts-speed');
    const fastttsSpeedVal = document.getElementById('fasttts-speed-val');
    const btnSynthFastTTS = document.getElementById('btn-synth-fasttts');
    const fastttsLoading = document.getElementById('fasttts-loading');
    const fastttsResult = document.getElementById('fasttts-result');
    const fastttsAudio = document.getElementById('fasttts-audio');

    if (fastttsSpeed) {
        fastttsSpeed.addEventListener('input', (e) => {
            fastttsSpeedVal.textContent = parseFloat(e.target.value).toFixed(1) + 'x';
        });
    }

    // ---- LOGIC THU GỌN / MỞ RỘNG INLINE (ACCORDION) ----
    const textPanelBody = document.getElementById('text-panel-body');
    const btnToggleTextPanel = document.getElementById('btn-toggle-text-panel');
    const iconToggleText = document.getElementById('icon-toggle-text');

    const btnToggleSettingsPanel = document.getElementById('btn-toggle-settings-panel');
    const iconToggleSettings = document.getElementById('icon-toggle-settings');

    function toggleTextPanel(expand) {
        if (!textPanelBody) return;
        const textPanelTools = document.getElementById('text-panel-tools');
        const panelContent = document.getElementById('panel-text-content');
        const isCurrentlyHidden = textPanelBody.style.display === 'none';
        const shouldExpand = expand !== undefined ? expand : isCurrentlyHidden;

        if (shouldExpand) {
            textPanelBody.style.display = '';
            if (btnToggleTextPanel) btnToggleTextPanel.classList.add('hidden');
            if (textPanelTools) textPanelTools.style.display = 'flex';
            if (panelContent) panelContent.classList.remove('collapsed');
        } else {
            textPanelBody.style.display = 'none';
            if (btnToggleTextPanel) btnToggleTextPanel.classList.remove('hidden');
            if (textPanelTools) textPanelTools.style.display = 'none';
            if (panelContent) panelContent.classList.add('collapsed');
        }
    }

    function toggleSettingsPanel(expand) {
        const tabContents = document.querySelectorAll('.tab-content');
        if (!tabContents.length) return;

        const firstActive = document.querySelector('.tab-content.active');
        const isCurrentlyHidden = firstActive && firstActive.style.display === 'none';
        const shouldExpand = expand !== undefined ? expand : isCurrentlyHidden;

        const panelSettings = document.getElementById('panel-settings-content');

        tabContents.forEach(tc => {
            if (shouldExpand) {
                tc.style.display = '';
            } else {
                tc.style.display = 'none';
            }
        });

        if (shouldExpand) {
            if (btnToggleSettingsPanel) btnToggleSettingsPanel.classList.add('hidden');
            if (panelSettings) panelSettings.classList.remove('collapsed');
        } else {
            if (btnToggleSettingsPanel) btnToggleSettingsPanel.classList.remove('hidden');
            if (panelSettings) panelSettings.classList.add('collapsed');
        }
    }

    if (btnToggleTextPanel) {
        btnToggleTextPanel.onclick = (e) => {
            e.stopPropagation();
            toggleTextPanel();
        };
    }

    if (btnToggleSettingsPanel) {
        btnToggleSettingsPanel.onclick = (e) => {
            e.stopPropagation();
            toggleSettingsPanel();
        };
    }

    function disableInactiveTabs(disable) {
        const tabBtns = document.querySelectorAll('.tab-btn');
        tabBtns.forEach(btn => {
            if (disable) {
                if (!btn.classList.contains('active')) {
                    btn.style.pointerEvents = 'none';
                    btn.style.opacity = '0.5';
                }
            } else {
                btn.style.pointerEvents = '';
                btn.style.opacity = '';
            }
        });
    }

    // ---- HỆ THỐNG TRÌNH PHÁT STREAMING CHUNK (AUDIO-BOOK) ----
    let streamingState = {
        active: false,
        cancelled: false,
        engine: 'fasttts',
        params: {},
        chunks: [],
        currentPlayIndex: -1,
        currentGenIndex: 0
    };

    const streamingPanel = document.getElementById('streaming-panel');
    const streamingChunkList = document.getElementById('streaming-chunk-list');
    const streamingStatusBadge = document.getElementById('streaming-status-badge');
    const streamingProgressText = document.getElementById('streaming-progress-text');
    const streamingAudioPlayer = document.getElementById('streaming-audio-player');
    
    if (streamingChunkList) {
        streamingChunkList.addEventListener('wheel', (evt) => {
            if (evt.deltaY !== 0) {
                evt.preventDefault();
                streamingChunkList.scrollLeft += evt.deltaY;
            }
        });
    }
    const currentPlayingText = document.getElementById('current-playing-text');
    const btnCancelStreaming = document.getElementById('btn-cancel-streaming');
    const btnResumeStreaming = document.getElementById('btn-resume-streaming');
    const btnDownloadCombinedMp3 = document.getElementById('btn-download-combined-mp3');
    const btnDownloadCombinedWav = document.getElementById('btn-download-combined-wav');
    const btnDownloadChunkMp3 = document.getElementById('btn-download-chunk-mp3');
    const btnDownloadChunkWav = document.getElementById('btn-download-chunk-wav');

    async function startStreamingPipeline(engineType, params) {
        const rawText = mainText.value.trim();
        if (!rawText) {
            showToast('Vui lòng nhập văn bản cần tổng hợp!', 'error');
            return;
        }

        currentJobId = crypto.randomUUID();
        const chunkTexts = splitTextIntoChunks(rawText, 1000, 2000);
        if (chunkTexts.length === 0) {
            showToast('Văn bản rỗng!', 'error');
            return;
        }

        // Tự động thu gọn nội dung 2 panel (Accordion) và vô hiệu hóa tab khác
        toggleTextPanel(false);
        toggleSettingsPanel(false);
        mainText.readOnly = true;
        mainText.style.opacity = 0.7;
        mainText.style.cursor = 'not-allowed';
        document.getElementById('custom-replace').disabled = true;
        document.getElementById('btn-custom-replace').disabled = true;
        disableInactiveTabs(true);

        // Hiện Panel Streaming Player
        if (streamingPanel) streamingPanel.classList.remove('hidden');
        if (btnDownloadCombinedMp3) btnDownloadCombinedMp3.classList.add('hidden');

        // Reset Trạng Thái
        streamingState = {
            active: true,
            cancelled: false,
            engine: engineType,
            params: params,
            chunks: [],
            currentPlayIndex: -1,
            currentGenIndex: 0,
            totalRetries: 0
        };

        const retrySpan = document.getElementById('streaming-retry-count-text');
        if (retrySpan) {
            retrySpan.style.display = 'none';
            retrySpan.innerHTML = `<i class="fa-solid fa-rotate-right"></i> 0 thử lại`;
        }

        if (btnDownloadChunkMp3) btnDownloadChunkMp3.classList.add('hidden');
        if (btnDownloadChunkWav) btnDownloadChunkWav.classList.add('hidden');
        if (btnDownloadCombinedMp3) btnDownloadCombinedMp3.classList.add('hidden');
        if (btnDownloadCombinedWav) btnDownloadCombinedWav.classList.add('hidden');

        if (btnCancelStreaming) {
            btnCancelStreaming.classList.remove('hidden');
            if (btnResumeStreaming) btnResumeStreaming.classList.add('hidden');
            btnCancelStreaming.innerHTML = '<i class="fa-solid fa-stop"></i> Dừng tiến trình';
            btnCancelStreaming.style.background = '#ef4444';
        }

        if (streamingChunkList) streamingChunkList.innerHTML = '';
        const fragment = document.createDocumentFragment();

        chunkTexts.forEach((ctext, i) => {
            const btn = document.createElement('button');
            btn.className = 'btn secondary-btn';
            btn.style.cssText = 'padding: 6px 14px; font-size: 0.85em; white-space: nowrap; flex-shrink: 0; border-radius: 6px; transition: all 0.2s ease; background: rgba(255,255,255,0.06); color: #94a3b8; border: 1px solid rgba(255,255,255,0.1); cursor: pointer;';
            let prevStr = ctext.substring(0, 25).replace(/\n/g, ' ').replace(/\s+/g, ' ').trim();
            btn.innerText = `Đoạn ${i + 1}: ${prevStr}...`;
            btn.title = `Bấm để nghe Đoạn ${i + 1}`;

            btn.onclick = () => {
                jumpToChunk(i);
            };

            const item = {
                index: i,
                text: ctext,
                status: 'pending',
                blob: null,
                blobUrl: null,
                btnEl: btn
            };
            streamingState.chunks.push(item);
            fragment.appendChild(btn);
        });

        if (streamingChunkList) streamingChunkList.appendChild(fragment);
        if (streamingProgressText) streamingProgressText.innerText = `0 / ${chunkTexts.length}`;
        if (streamingStatusBadge) {
            streamingStatusBadge.innerText = 'Đang khởi chạy...';
            streamingStatusBadge.style.background = 'rgba(99, 102, 241, 0.2)';
            streamingStatusBadge.style.color = '#a5b4fc';
        }

        
        // Khởi tạo Job qua API History trước
        try {
            const initRes = await fetch('/api/jobs/init', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    job_id: currentJobId,
                    engine: engineType,
                    voice: params.voice || params.clone_id || '',
                    speed: params.speed || 1.0,
                    total_chunks: chunkTexts.length,
                    text: rawText
                })
            });
            // If not logged in, it might fail, but we can ignore or let it pass if Auth is not strict on this endpoint.
        } catch (e) {
            console.error(e);
        }

        // Chạy vòng lặp tổng hợp cuốn chiếu
        generateChunksLoop();

    }

    function updateChunkUI(index) {
        const item = streamingState.chunks[index];
        if (!item || !item.btnEl) return;

        if (item.status === 'playing') {
            item.btnEl.style.background = '#10b981'; // Màu Xanh Lá khi ĐANG PHÁT
            item.btnEl.style.color = '#ffffff';
            item.btnEl.style.borderColor = '#059669';
        } else if (item.status === 'ready') {
            item.btnEl.style.background = '#f59e0b'; // Màu Vàng khi ĐÃ TẢI XONG
            item.btnEl.style.color = '#1e293b';
            item.btnEl.style.borderColor = '#d97706';
        } else if (item.status === 'error') {
            item.btnEl.style.background = '#ef4444'; // Màu Đỏ nếu LỖI
            item.btnEl.style.color = '#ffffff';
            item.btnEl.style.borderColor = '#b91c1c';
        } else if (item.status === 'retrying') {
            item.btnEl.style.background = '#a855f7'; // Màu Tím khi ĐANG THỬ LẠI
            item.btnEl.style.color = '#ffffff';
            item.btnEl.style.borderColor = '#7e22ce';
        } else {
            item.btnEl.style.background = 'rgba(255,255,255,0.06)'; // Mặc định chưa làm
            item.btnEl.style.color = '#94a3b8';
            item.btnEl.style.borderColor = 'rgba(255,255,255,0.1)';
        }
    }

    async function generateChunksLoop() {
        for (let i = 0; i < streamingState.chunks.length; i++) {
            if (!streamingState.active || streamingState.cancelled) break;

            streamingState.currentGenIndex = i;
            const item = streamingState.chunks[i];
            
            if (item.status === 'ready' || item.status === 'done') continue;
            
            if (streamingStatusBadge) {
                streamingStatusBadge.innerText = `Đang tạo Đoạn ${i + 1}/${streamingState.chunks.length}...`;
            }

            let success = false;
            let lastError = null;

            for (let attempt = 1; attempt <= 3; attempt++) {
                if (!streamingState.active || streamingState.cancelled) break;

                try {
                    if (attempt > 1) {
                        streamingState.totalRetries++;
                        const retrySpan = document.getElementById('streaming-retry-count-text');
                        if (retrySpan) {
                            retrySpan.style.display = 'inline';
                            retrySpan.innerHTML = `<i class="fa-solid fa-rotate-right"></i> ${streamingState.totalRetries} thử lại`;
                        }

                        item.status = 'retrying';
                        updateChunkUI(i);
                        if (streamingStatusBadge) {
                            streamingStatusBadge.innerText = `Đang thử lại Đoạn ${i + 1}/${streamingState.chunks.length} (Lần ${attempt}/3)...`;
                            streamingStatusBadge.style.background = 'rgba(168, 85, 247, 0.2)';
                            streamingStatusBadge.style.color = '#d8b4fe';
                        }
                        await new Promise(r => setTimeout(r, 2000)); // Nghỉ 2s trước khi thử lại
                    }

                    const blob = await requestChunkAudio(
                        item.text, 
                        streamingState.engine, 
                        streamingState.params,
                        currentJobId,
                        i,
                        streamingState.chunks.length
                    );
                    if (!streamingState.active || streamingState.cancelled) break;

                    item.blob = blob;
                    item.blobUrl = URL.createObjectURL(blob);
                    item.status = 'ready';
                    updateChunkUI(i);
                    success = true;
                    break;
                } catch (err) {
                    lastError = err;
                    console.error(`Lỗi tạo chunk ${i + 1} (Lần ${attempt}):`, err);
                }
            }

            if (!streamingState.active || streamingState.cancelled) break;

            if (!success) {
                item.status = 'error';
                updateChunkUI(i);
                showToast(`Đoạn ${i + 1} thất bại sau 3 lần thử: ${lastError ? lastError.message : 'Lỗi không xác định'}`, 'error');
            } else {
                const readyCount = streamingState.chunks.filter(c => c.status === 'ready' || c.status === 'playing').length;
                if (streamingProgressText) {
                    streamingProgressText.innerText = `${readyCount} / ${streamingState.chunks.length}`;
                }

                // Tự động phát khi đoạn đầu tiên tải xong
                if (streamingState.currentPlayIndex === -1 && i === 0) {
                    playChunk(0);
                } else if (streamingState.currentPlayIndex !== -1 && streamingAudioPlayer.paused && streamingAudioPlayer.ended) {
                    // Nếu player bị khựng do chờ tải tiếp
                    if (streamingState.currentPlayIndex + 1 === i) {
                        playChunk(i);
                    }
                }
            }
        }

        if (!streamingState.cancelled) {
            if (streamingStatusBadge) {
                streamingStatusBadge.innerText = 'Hoàn tất toàn bộ!';
                streamingStatusBadge.style.background = 'rgba(16, 185, 129, 0.2)';
                streamingStatusBadge.style.color = '#34d399';
            }

            if (btnCancelStreaming) {
                btnCancelStreaming.classList.add('hidden'); // Ẩn hoàn toàn nút dừng khi đã xong
            }

            // Hiện nút tải toàn bộ MP3/WAV nối sẵn
            const readyBlobs = streamingState.chunks.filter(c => c.blob).map(c => c.blob);
            if (readyBlobs.length > 0) {
                if (btnDownloadCombinedMp3) {
                    btnDownloadCombinedMp3.classList.remove('hidden');
                    btnDownloadCombinedMp3.onclick = () => {
                        const combinedBlob = new Blob(readyBlobs, { type: 'audio/mp3' });
                        const url = URL.createObjectURL(combinedBlob);
                        const a = document.createElement('a');
                        a.href = url;
                        a.download = `full_tts_combined_${Date.now()}.mp3`;
                        a.click();
                    };
                }
                if (btnDownloadCombinedWav && streamingState.engine !== 'fasttts') {
                    btnDownloadCombinedWav.classList.remove('hidden');
                    btnDownloadCombinedWav.onclick = () => {
                        const combinedBlob = new Blob(readyBlobs, { type: 'audio/wav' });
                        const url = URL.createObjectURL(combinedBlob);
                        const a = document.createElement('a');
                        a.href = url;
                        a.download = `full_tts_combined_${Date.now()}.wav`;
                        a.click();
                    };
                }
            }
        }
        
        // Hoàn tất hoặc bị hủy thì bật lại các tab
        disableInactiveTabs(false);
    }

    function playChunk(index) {
        if (index < 0 || index >= streamingState.chunks.length) return;
        const item = streamingState.chunks[index];
        if (item.status !== 'ready' && item.status !== 'playing') return;

        // Trả lại màu Vàng cho chunk trước đó nếu nó vừa phát xong
        if (streamingState.currentPlayIndex !== -1 && streamingState.currentPlayIndex !== index) {
            const prevItem = streamingState.chunks[streamingState.currentPlayIndex];
            if (prevItem && prevItem.status === 'playing') {
                prevItem.status = 'ready';
                updateChunkUI(streamingState.currentPlayIndex);
            }
        }

        streamingState.currentPlayIndex = index;
        item.status = 'playing';
        updateChunkUI(index);

        let prevText = item.text.substring(0, 45).replace(/\n/g, ' ').replace(/\s+/g, ' ').trim();
        if (currentPlayingText) {
            currentPlayingText.innerText = `Đoạn ${index + 1}: "${prevText}..."`;
        }

        if (streamingAudioPlayer) {
            streamingAudioPlayer.src = item.blobUrl;
            streamingAudioPlayer.play().catch(e => console.warn('Không thể tự động phát:', e));
        }

        if (btnDownloadChunkMp3 && item.blobUrl) {
            btnDownloadChunkMp3.href = item.blobUrl;
            btnDownloadChunkMp3.download = `chunk_${index + 1}_${Date.now()}.mp3`;
            btnDownloadChunkMp3.classList.remove('hidden');
        }
        if (btnDownloadChunkWav && item.blobUrl && streamingState.engine !== 'fasttts') {
            btnDownloadChunkWav.href = item.blobUrl;
            btnDownloadChunkWav.download = `chunk_${index + 1}_${Date.now()}.wav`;
            btnDownloadChunkWav.classList.remove('hidden');
        }

        updateChunkUI(index);

        // Tự cuộn băng chuyền chunk tới đoạn đang phát
        item.btnEl.scrollIntoView({ behavior: 'smooth', block: 'nearest', inline: 'center' });
    }

    function jumpToChunk(index) {
        const item = streamingState.chunks[index];
        if (item.status === 'ready' || item.status === 'playing') {
            playChunk(index);
        } else {
            showToast(`Đoạn ${index + 1} chưa được tạo xong! V vui lòng chờ...`, 'info');
        }
    }

    // Khi phát hết 1 chunk -> Tự phát chunk tiếp theo
    if (streamingAudioPlayer) {
        streamingAudioPlayer.onended = () => {
            const nextIndex = streamingState.currentPlayIndex + 1;
            if (nextIndex < streamingState.chunks.length) {
                const nextItem = streamingState.chunks[nextIndex];
                if (nextItem.status === 'ready') {
                    playChunk(nextIndex);
                } else {
                    if (streamingStatusBadge) {
                        streamingStatusBadge.innerText = `Đang chờ Đoạn ${nextIndex + 1} tải về...`;
                    }
                }
            } else {
                if (currentPlayingText) {
                    currentPlayingText.innerText = 'Đã phát hết toàn bộ văn bản!';
                }
            }
        };
    }

    // Nút Dừng tiến trình
    if (btnCancelStreaming) {
        btnCancelStreaming.onclick = () => {
            streamingState.cancelled = true;
            streamingState.active = false;
            disableInactiveTabs(false);
            if (streamingAudioPlayer) streamingAudioPlayer.pause();
            if (streamingStatusBadge) {
                streamingStatusBadge.innerText = 'Đã dừng!';
                streamingStatusBadge.style.background = 'rgba(239, 68, 68, 0.2)';
                streamingStatusBadge.style.color = '#f87171';
            }
            showToast('Đã dừng tiến trình Streaming!', 'info');
            
            if (btnResumeStreaming && streamingState.chunks.some(c => c.status === 'pending' || c.status === 'error')) {
                btnCancelStreaming.classList.add('hidden');
                btnResumeStreaming.classList.remove('hidden');
            }
        };
    }
    
    // Nút Tiếp tục tiến trình
    if (btnResumeStreaming) {
        btnResumeStreaming.onclick = () => {
            btnResumeStreaming.classList.add('hidden');
            btnCancelStreaming.classList.remove('hidden');
            streamingState.cancelled = false;
            streamingState.active = true;
            generateChunksLoop();
        };
    }

    async function requestChunkAudio(text, engine, params, jobId, chunkIndex, totalChunks) {
        let endpoint = '/api/fasttts/synthesize';
        let body = { 
            text, 
            voice: params.voice || 'Hoài Mỹ (Nữ)', 
            speed: params.speed || 1.0,
            job_id: jobId,
            chunk_index: chunkIndex,
            total_chunks: totalChunks
        };

        if (engine === 'standard') {
            endpoint = '/api/standard/synthesize';
            body = { 
                text, 
                voice: params.voice, 
                speed: params.speed,
                job_id: jobId,
                chunk_index: chunkIndex,
                total_chunks: totalChunks
            };
        } else if (engine === 'clone') {
            endpoint = '/api/clone/synthesize';
            body = { 
                text, 
                clone_id: params.cloneId, 
                speed: params.speed,
                job_id: jobId,
                chunk_index: chunkIndex,
                total_chunks: totalChunks
            };
        }

        const res = await fetch(endpoint, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body)
        });

        if (!res.ok) {
            const err = await res.json();
            throw new Error(err.detail || 'Khởi tạo tác vụ thất bại');
        }

        const data = await res.json();
        const taskId = data.task_id;

        return new Promise((resolve, reject) => {
            const evtSource = new EventSource(`/api/stream/tasks/${taskId}`);

            evtSource.onmessage = async (event) => {
                if (streamingState.cancelled) {
                    evtSource.close();
                    fetch(`/api/tasks/${taskId}/cancel`, { method: 'POST' }).catch(() => {});
                    reject(new Error('Đã hủy'));
                    return;
                }

                try {
                    const eventData = JSON.parse(event.data);
                    if (eventData.status === 'cancelled') {
                        evtSource.close();
                        reject(new Error('Tác vụ bị hủy'));
                    } else if (eventData.status === 'error') {
                        evtSource.close();
                        reject(new Error(eventData.error || 'Lỗi AI'));
                    } else if (eventData.status === 'done') {
                        evtSource.close();
                        const audioRes = await fetch(`/api/tasks/${taskId}/audio?format=mp3`);
                        if (!audioRes.ok) throw new Error('Không thể tải file âm thanh');
                        const blob = await audioRes.blob();
                        resolve(blob);
                    }
                } catch (e) {
                    evtSource.close();
                    reject(e);
                }
            };

            evtSource.onerror = () => {
                evtSource.close();
                reject(new Error('Lỗi kết nối Server'));
            };
        });
    }

    if (btnSynthFastTTS) {
        btnSynthFastTTS.addEventListener('click', async () => {
            const selectedRadio = document.querySelector('input[name="fasttts-voice-radio"]:checked');
            const voiceVal = selectedRadio ? selectedRadio.value : 'Hoài Mỹ (Nữ)';
            const speedVal = parseFloat(fastttsSpeed ? fastttsSpeed.value : 1.0);
            startStreamingPipeline('fasttts', { voice: voiceVal, speed: speedVal });
        });
    }

    if (btnSynthesize) {
        btnSynthesize.addEventListener('click', () => {
            executeWithWarning(() => {
                const voiceVal = stdVoice.value;
                const speedVal = parseFloat(stdSpeed ? stdSpeed.value : 1.0);
                startStreamingPipeline('standard', { voice: voiceVal, speed: speedVal });
            });
        });
    }

    if (btnSynthClone) {
        btnSynthClone.addEventListener('click', () => {
            executeWithWarning(async () => {
                if (!currentCloneId) {
                    if (currentAudioBuffer && originalFile) {
                        let fileToUpload = trimmedFileToUpload;
                        if (!fileToUpload) {
                            if (currentAudioBuffer.duration <= 5.0) {
                                fileToUpload = originalFile;
                            } else {
                                const wavBlob = audioBufferToWav(currentAudioBuffer, trimStartNum, trimEndNum);
                                const newFileName = originalFile.name.replace(/\.[^/.]+$/, "") + "_trimmed.wav";
                                fileToUpload = new File([wavBlob], newFileName, { type: 'audio/wav' });
                            }
                        }
                        const oldText = btnSynthClone.innerHTML;
                        btnSynthClone.disabled = true;
                        btnSynthClone.innerHTML = '<i class="fa-solid fa-spinner fa-spin"></i> Đang tải lên server...';
                        const success = await uploadCloneFile(fileToUpload);
                        btnSynthClone.disabled = false;
                        btnSynthClone.innerHTML = oldText;
                        if (!success || !currentCloneId) return;
                    } else {
                        showToast('Vui lòng chọn file âm thanh giọng Clone trước', 'error');
                        return;
                    }
                }
                const speedVal = parseFloat(cloneSpeed ? cloneSpeed.value : 1.0);
                startStreamingPipeline('clone', { speed: speedVal, cloneId: currentCloneId });
            });
        });
    }

    function showToast(message, type = 'success') {
        const container = document.getElementById('toast-container');
        const toast = document.createElement('div');
        toast.className = `toast ${type}`;
        
        const icon = type === 'success' ? 'fa-circle-check' : 'fa-circle-xmark';
        toast.innerHTML = `<i class="fa-solid ${icon}"></i> <span>${message}</span>`;
        
        container.appendChild(toast);
        
        setTimeout(() => {
            toast.style.animation = 'slideIn 0.3s ease reverse forwards';
            setTimeout(() => toast.remove(), 300);
        }, 3000);
    }
    
    // Helper function: Tính toán chính xác vị trí pixel (offsetTop) của 1 index trong textarea
    function getScrollPositionOfIndex(element, position) {
        const div = document.createElement('div');
        const style = window.getComputedStyle(element);
        
        // Copy toàn bộ style ảnh hưởng đến bố cục chữ
        div.style.whiteSpace = 'pre-wrap';
        div.style.wordWrap = 'break-word';
        div.style.position = 'absolute';
        div.style.visibility = 'hidden';
        
        // CỰC KỲ QUAN TRỌNG: Lấy clientWidth (chiều rộng lọt lòng đã trừ thanh cuộn)
        // Nếu lấy style.width, bóng div sẽ không có thanh cuộn -> rộng hơn textarea thực tế -> rớt dòng sai
        div.style.width = element.clientWidth + 'px';
        div.style.boxSizing = 'border-box';
        
        div.style.fontFamily = style.fontFamily;
        div.style.fontSize = style.fontSize;
        div.style.lineHeight = style.lineHeight;
        div.style.padding = style.padding;
        div.style.border = 'none'; // Không cần border vì clientWidth đã loại trừ nó
        
        // Đổ văn bản từ đầu đến vị trí cần tìm
        div.textContent = element.value.substring(0, position);
        
        // Đặt 1 thẻ span chốt chặn để lấy tọa độ
        const span = document.createElement('span');
        span.textContent = element.value.substring(position, position + 1) || '.';
        div.appendChild(span);
        
        document.body.appendChild(div);
        const top = span.offsetTop;
        document.body.removeChild(div);
        
        return top;
    }
//===========================================
// AUTH & ADMIN LOGIC
//===========================================
async function checkAuthStatus() {
    try {
        const res = await fetch('/api/me');
        if (res.ok) {
            const user = await res.json();
            document.getElementById('auth-overlay').style.display = 'none';
            document.getElementById('btn-logout').style.display = 'block';
            document.getElementById('btn-show-history').style.display = 'block';
            if (user.role === 'admin') {
                document.getElementById('btn-show-admin').style.display = 'block';
            }
        } else {
            document.getElementById('auth-overlay').style.display = 'flex';
        }
    } catch(e) {
        document.getElementById('auth-overlay').style.display = 'flex';
    }
}
checkAuthStatus();

document.getElementById('link-show-register')?.addEventListener('click', (e) => {
    e.preventDefault();
    document.getElementById('login-form').style.display = 'none';
    document.getElementById('register-form').style.display = 'block';
});

document.getElementById('link-show-login')?.addEventListener('click', (e) => {
    e.preventDefault();
    document.getElementById('register-form').style.display = 'none';
    document.getElementById('login-form').style.display = 'block';
});

document.getElementById('btn-login-submit')?.addEventListener('click', async () => {
    const u = document.getElementById('login-username').value;
    const p = document.getElementById('login-password').value;
    const formData = new URLSearchParams();
    formData.append('username', u);
    formData.append('password', p);
    try {
        const res = await fetch('/api/login', { method: 'POST', body: formData, headers: {'Content-Type': 'application/x-www-form-urlencoded'} });
        if (!res.ok) {
            const data = await res.json();
            showToast(data.detail || 'Đăng nhập thất bại', 'error');
            return;
        }
        const data = await res.json();
        // Cơ chế HttpOnly Cookie đã xử lý xác thực an toàn, không cần fake localStorage
        window.location.reload();
    } catch(e) {
        showToast(e.message || 'Lỗi đăng nhập', 'error');
    }
});

document.getElementById('btn-logout')?.addEventListener('click', async () => {
    await fetch('/api/logout', { method: 'POST' });
    window.location.reload();
});

    // Clone File Upload Handling
    uploadArea.addEventListener('click', (e) => {
        if (e.target.tagName.toLowerCase() === 'input') return;
        cloneFile.value = ''; // Reset input to allow selecting same file again
        cloneFile.click();
    });
    
    uploadArea.addEventListener('dragover', (e) => {
        e.preventDefault();
        uploadArea.classList.add('dragover');
    });
    
    uploadArea.addEventListener('dragleave', () => {
        uploadArea.classList.remove('dragover');
    });
    
    uploadArea.addEventListener('drop', (e) => {
        e.preventDefault();
        uploadArea.classList.remove('dragover');
        if (e.dataTransfer.files.length) {
            cloneFile.files = e.dataTransfer.files;
            handleFileSelect();
        }
    });

    cloneFile.addEventListener('change', handleFileSelect);

    const audioContext = new (window.AudioContext || window.webkitAudioContext)();
    let currentAudioBuffer = null;
    let originalFile = null;
    let trimmedFileToUpload = null;

    async function handleFileSelect() {
        if (audioContext.state === 'suspended') {
            await audioContext.resume();
        }
        
        if (cloneFile.files.length > 0) {
            const file = cloneFile.files[0];

            if (!file.name.toLowerCase().endsWith('.wav')) {
                showToast('Chỉ chấp nhận định dạng file .wav!', 'error');
                uploadAreaText.innerHTML = `Lỗi: Định dạng không hỗ trợ. Kéo thả file .wav vào đây.`;
                uploadArea.style.pointerEvents = 'auto';
                cloneFile.value = '';
                return;
            }

            originalFile = file;
            if (btnSynthClone) btnSynthClone.disabled = true;
            uploadAreaText.innerHTML = `Đang tải: <i class="fa-solid fa-spinner fa-spin"></i> <span>${file.name}</span>...`;
            uploadArea.style.pointerEvents = 'none';
            
            try {
                const arrayBuffer = await file.arrayBuffer();
                currentAudioBuffer = await audioContext.decodeAudioData(arrayBuffer);
                
                if (currentAudioBuffer.duration > 30.0) {
                    showToast('Vui lòng chọn file âm thanh ngắn hơn 30 giây!', 'error');
                    uploadAreaText.innerHTML = `Lỗi: File gốc quá dài (>30s). Kéo thả file khác.`;
                    uploadArea.style.pointerEvents = 'auto';
                    cloneFile.value = '';
                    return;
                }
                
                if (currentAudioBuffer.duration <= 5.0) {
                    if (btnSynthClone) btnSynthClone.disabled = false;
                } else {
                    setupTrimmer(currentAudioBuffer.duration, file);
                }
            } catch (error) {
                showToast('Lỗi đọc file âm thanh!', 'error');
                uploadAreaText.innerHTML = `Kéo thả file âm thanh hoặc <span>chọn file</span>`;
                uploadArea.style.pointerEvents = 'auto';
            }
        }
    }

    let trimStartNum = 0;
    let trimEndNum = 0;
    const TRIM_LENGTH = 5.0;

    function setupTrimmer(duration, file) {
        trimmerDiv.classList.remove('hidden');
        uploadAreaText.innerHTML = `<i class="fa-solid fa-check text-success"></i> Đã tải file: <span>${file.name}</span>. Vui lòng cắt bên dưới.`;
        
        trimStartNum = 0;
        trimEndNum = TRIM_LENGTH;
        
        const updateVals = () => {
            trimStartVal.textContent = trimStartNum.toFixed(1);
            trimEndVal.textContent = trimEndNum.toFixed(1);
            trimDurationVal.textContent = (trimEndNum - trimStartNum).toFixed(1);
        };
        updateVals();
        
        // Draw Waveform
        const ctx = waveformCanvas.getContext('2d');
        const data = currentAudioBuffer.getChannelData(0);
        const step = Math.ceil(data.length / waveformCanvas.width);
        const amp = waveformCanvas.height / 2;
        
        // Precalculate waveform peaks ONCE (this prevents lag during mouse drag!)
        const wavePeaks = [];
        for (let i = 0; i < waveformCanvas.width; i++) {
            let min = 1.0;
            let max = -1.0;
            for (let j = 0; j < step; j++) {
                const datum = data[i * step + j] || 0;
                if (datum < min) min = datum;
                if (datum > max) max = datum;
            }
            wavePeaks.push({ min, max });
        }
        
        const drawWaveform = () => {
            ctx.clearRect(0, 0, waveformCanvas.width, waveformCanvas.height);
            
            // Draw Waves from precalculated peaks
            ctx.fillStyle = 'rgba(99, 102, 241, 0.5)';
            for (let i = 0; i < waveformCanvas.width; i++) {
                const peak = wavePeaks[i];
                ctx.fillRect(i, (1 + peak.min) * amp, 1, Math.max(1, (peak.max - peak.min) * amp));
            }
            
            // Draw Selection Overlay
            const startX = (trimStartNum / duration) * waveformCanvas.width;
            const endX = (trimEndNum / duration) * waveformCanvas.width;
            
            ctx.fillStyle = 'rgba(0, 0, 0, 0.6)';
            ctx.fillRect(0, 0, startX, waveformCanvas.height); // dark left
            ctx.fillRect(endX, 0, waveformCanvas.width - endX, waveformCanvas.height); // dark right
            
            // Draw Edges
            ctx.fillStyle = '#10b981'; // green edges
            ctx.fillRect(startX - 2, 0, 4, waveformCanvas.height);
            ctx.fillRect(endX - 2, 0, 4, waveformCanvas.height);
        };
        
        drawWaveform();
        
        // Handle Canvas Drag Events
        let isDragging = false;
        let drawPending = false;
        
        const scheduleDraw = () => {
            if (!drawPending) {
                drawPending = true;
                requestAnimationFrame(() => {
                    updateVals();
                    drawWaveform();
                    drawPending = false;
                });
            }
        };
        
        const updateWindowPos = (e) => {
            const rect = waveformCanvas.getBoundingClientRect();
            const scaleX = waveformCanvas.width / rect.width;
            const x = (e.clientX - rect.left) * scaleX;
            const timeClicked = (x / waveformCanvas.width) * duration;
            
            // Center the 5s window
            let start = timeClicked - (TRIM_LENGTH / 2);
            let end = timeClicked + (TRIM_LENGTH / 2);
            
            if (start < 0) {
                start = 0;
                end = TRIM_LENGTH;
            }
            if (end > duration) {
                end = duration;
                start = duration - TRIM_LENGTH;
            }
            
            trimStartNum = start;
            trimEndNum = end;
            scheduleDraw();
        };

        waveformCanvas.onmousedown = (e) => {
            isDragging = true;
            updateWindowPos(e);
        };
        
        window.onmousemove = (e) => {
            if (!isDragging) return;
            updateWindowPos(e);
        };
        
        window.onmouseup = () => isDragging = false;
        
        // Reset state
        trimmedFileToUpload = null;
        btnTrimUpload.disabled = false;
        if (btnSynthClone) btnSynthClone.disabled = false;
        trimPreviewAudio.src = URL.createObjectURL(file);
        uploadArea.style.pointerEvents = 'auto';
    }

    if (btnTrimOnly) {
        btnTrimOnly.addEventListener('click', () => {
            if (!currentAudioBuffer) return;
            
            const wavBlob = audioBufferToWav(currentAudioBuffer, trimStartNum, trimEndNum);
            const newFileName = originalFile.name.replace(/\.[^/.]+$/, "") + "_trimmed.wav";
            trimmedFileToUpload = new File([wavBlob], newFileName, { type: 'audio/wav' });
            
            // Update audio preview to the trimmed version
            trimPreviewAudio.src = URL.createObjectURL(trimmedFileToUpload);
            trimPreviewAudio.play();
            
            // Enable upload button
            if (btnTrimUpload) btnTrimUpload.disabled = false;
            showToast('Đã cắt xong, vui lòng nghe lại và bấm Tải lên!', 'success');
        });
    }

    if (btnTrimUpload) {
        btnTrimUpload.addEventListener('click', () => {
            if (!trimmedFileToUpload) return;
            
            btnTrimUpload.disabled = true;
            btnTrimUpload.innerHTML = '<i class="fa-solid fa-spinner fa-spin"></i> Đang tải lên...';
            
            uploadCloneFile(trimmedFileToUpload);
        });
    }

    const createVoiceModal = document.getElementById('create-voice-modal');
    const btnOpenCreateVoiceModal = document.getElementById('btn-open-create-voice-modal');
    const btnCloseCreateVoiceModal = document.getElementById('btn-close-create-voice-modal');
    const btnModalCloseVoice = document.getElementById('btn-modal-close-voice');
    const btnModalTrimUpload = document.getElementById('btn-modal-trim-upload');
    const modalCloneFile = document.getElementById('modal-clone-file');

    function openCreateVoiceModal() {
        if (createVoiceModal) createVoiceModal.classList.remove('hidden');
    }

    function hideCreateVoiceModal() {
        if (createVoiceModal) createVoiceModal.classList.add('hidden');
        if (modalCloneFile) modalCloneFile.value = '';
        const modalTrimmerDiv = document.getElementById('modal-audio-trimmer');
        if (modalTrimmerDiv) modalTrimmerDiv.classList.add('hidden');
        const modalUploadAreaText = document.getElementById('modal-upload-area-text');
        if (modalUploadAreaText) modalUploadAreaText.innerHTML = `Kéo thả file âm thanh .wav hoặc <span>chọn file</span>`;
    }

    if (btnOpenCreateVoiceModal) btnOpenCreateVoiceModal.addEventListener('click', openCreateVoiceModal);
    if (btnCloseCreateVoiceModal) btnCloseCreateVoiceModal.addEventListener('click', hideCreateVoiceModal);
    if (btnModalCloseVoice) btnModalCloseVoice.addEventListener('click', hideCreateVoiceModal);

    if (btnTrimOnly) {
        btnTrimOnly.addEventListener('click', () => {
            if (!currentAudioBuffer) return;
            
            const wavBlob = audioBufferToWav(currentAudioBuffer, trimStartNum, trimEndNum);
            const newFileName = originalFile.name.replace(/\.[^/.]+$/, "") + "_trimmed.wav";
            trimmedFileToUpload = new File([wavBlob], newFileName, { type: 'audio/wav' });
            
            // Update audio preview to the trimmed version
            trimPreviewAudio.src = URL.createObjectURL(trimmedFileToUpload);
            trimPreviewAudio.play();
            
            if (btnSynthClone) btnSynthClone.disabled = false;
            showToast('Đã cắt 5s thành công! Bấm "Tổng hợp âm thanh" bên dưới để đọc.', 'success');
        });
    }

    async function uploadCloneFile(file) {
        const formData = new FormData();
        formData.append('file', file);
        
        try {
            const response = await fetch('/api/clone/upload-temp', {
                method: 'POST',
                body: formData
            });

            const data = await response.json();
            if (!response.ok) throw new Error(data.detail || 'Lỗi server');

            currentCloneId = data.clone_id;
            cloneIdDisplay.textContent = currentCloneId;
            cloneStatus.classList.remove('hidden');
            if (btnSynthClone) btnSynthClone.disabled = false;
            return true;
        } catch (error) {
            showToast('Lỗi: ' + error.message, 'error');
            return false;
        }
    }

    if (btnModalTrimUpload) {
        btnModalTrimUpload.addEventListener('click', () => {
            const fileToUpload = modalTrimmedFileToUpload || modalOriginalFile;
            if (!fileToUpload) return;
            
            btnModalTrimUpload.disabled = true;
            btnModalTrimUpload.innerHTML = '<i class="fa-solid fa-spinner fa-spin"></i> Đang tải lên...';
            
            uploadCloneFileModal(fileToUpload);
        });
    }

    let modalOriginalFile = null;
    let modalAudioBuffer = null;
    let modalTrimmedFileToUpload = null;
    let modalTrimStartNum = 0;
    let modalTrimEndNum = 0;

    const btnModalTrimOnly = document.getElementById('btn-modal-trim-only');
    const modalWaveformCanvas = document.getElementById('modal-waveform-canvas');
    const modalTrimStartVal = document.getElementById('modal-trim-start-val');
    const modalTrimEndVal = document.getElementById('modal-trim-end-val');
    const modalTrimDurationVal = document.getElementById('modal-trim-duration-val');
    const modalTrimPreviewAudio = document.getElementById('modal-trim-preview-audio');
    const modalTrimmerDiv = document.getElementById('modal-audio-trimmer');

    if (modalCloneFile) {
        modalCloneFile.addEventListener('change', async (e) => {
            const file = e.target.files[0];
            if (!file) return;
            if (!file.name.toLowerCase().endsWith('.wav')) {
                showToast('Chỉ chấp nhận file âm thanh định dạng .wav!', 'error');
                modalCloneFile.value = '';
                return;
            }
            modalOriginalFile = file;
            modalTrimmedFileToUpload = null;
            
            const modalUploadAreaText = document.getElementById('modal-upload-area-text');
            if (modalUploadAreaText) modalUploadAreaText.innerHTML = `<i class="fa-solid fa-file-audio text-primary"></i> ${file.name}`;
            
            try {
                const arrayBuffer = await file.arrayBuffer();
                const audioCtx = new (window.AudioContext || window.webkitAudioContext)();
                modalAudioBuffer = await audioCtx.decodeAudioData(arrayBuffer);
                
                if (modalAudioBuffer.duration <= 5.0) {
                    if (modalTrimmerDiv) modalTrimmerDiv.classList.add('hidden');
                    if (btnModalTrimUpload) btnModalTrimUpload.disabled = false;
                } else {
                    setupModalTrimmer(modalAudioBuffer.duration, file);
                }
            } catch (err) {
                showToast('Lỗi đọc file âm thanh!', 'error');
            }
        });
    }

    function setupModalTrimmer(duration, file) {
        if (!modalTrimmerDiv || !modalWaveformCanvas) return;
        modalTrimmerDiv.classList.remove('hidden');
        if (btnModalTrimUpload) btnModalTrimUpload.disabled = false;
        
        modalTrimStartNum = 0;
        modalTrimEndNum = 5.0;
        
        const updateVals = () => {
            if (modalTrimStartVal) modalTrimStartVal.textContent = modalTrimStartNum.toFixed(1);
            if (modalTrimEndVal) modalTrimEndVal.textContent = modalTrimEndNum.toFixed(1);
            if (modalTrimDurationVal) modalTrimDurationVal.textContent = (modalTrimEndNum - modalTrimStartNum).toFixed(1);
        };
        updateVals();
        
        const ctx = modalWaveformCanvas.getContext('2d');
        const data = modalAudioBuffer.getChannelData(0);
        const step = Math.ceil(data.length / modalWaveformCanvas.width);
        const amp = modalWaveformCanvas.height / 2;
        
        const wavePeaks = [];
        for (let i = 0; i < modalWaveformCanvas.width; i++) {
            let min = 1.0;
            let max = -1.0;
            for (let j = 0; j < step; j++) {
                const datum = data[i * step + j] || 0;
                if (datum < min) min = datum;
                if (datum > max) max = datum;
            }
            wavePeaks.push({ min, max });
        }
        
        const drawWaveform = () => {
            ctx.clearRect(0, 0, modalWaveformCanvas.width, modalWaveformCanvas.height);
            ctx.fillStyle = 'rgba(168, 85, 247, 0.5)';
            for (let i = 0; i < modalWaveformCanvas.width; i++) {
                const peak = wavePeaks[i];
                ctx.fillRect(i, (1 + peak.min) * amp, 1, Math.max(1, (peak.max - peak.min) * amp));
            }
            
            const startX = (modalTrimStartNum / duration) * modalWaveformCanvas.width;
            const endX = (modalTrimEndNum / duration) * modalWaveformCanvas.width;
            
            ctx.fillStyle = 'rgba(0, 0, 0, 0.6)';
            ctx.fillRect(0, 0, startX, modalWaveformCanvas.height);
            ctx.fillRect(endX, 0, modalWaveformCanvas.width - endX, modalWaveformCanvas.height);
            
            ctx.fillStyle = '#a855f7';
            ctx.fillRect(startX - 2, 0, 4, modalWaveformCanvas.height);
            ctx.fillRect(endX - 2, 0, 4, modalWaveformCanvas.height);
        };
        
        drawWaveform();
        
        let isDragging = false;
        let drawPending = false;
        
        const updateWindowPos = (e) => {
            const rect = modalWaveformCanvas.getBoundingClientRect();
            const scaleX = modalWaveformCanvas.width / rect.width;
            const x = (e.clientX - rect.left) * scaleX;
            const timeClicked = (x / modalWaveformCanvas.width) * duration;
            
            let start = timeClicked - 2.5;
            let end = timeClicked + 2.5;
            
            if (start < 0) { start = 0; end = 5.0; }
            if (end > duration) { end = duration; start = duration - 5.0; }
            
            modalTrimStartNum = start;
            modalTrimEndNum = end;
            
            if (!drawPending) {
                drawPending = true;
                requestAnimationFrame(() => {
                    updateVals();
                    drawWaveform();
                    drawPending = false;
                });
            }
        };

        modalWaveformCanvas.onmousedown = (e) => { isDragging = true; updateWindowPos(e); };
        window.addEventListener('mousemove', (e) => { if (isDragging) updateWindowPos(e); });
        window.addEventListener('mouseup', () => { isDragging = false; });
    }

    if (btnModalTrimOnly) {
        btnModalTrimOnly.addEventListener('click', () => {
            if (!modalAudioBuffer || !modalOriginalFile) return;
            const wavBlob = audioBufferToWav(modalAudioBuffer, modalTrimStartNum, modalTrimEndNum);
            const newFileName = modalOriginalFile.name.replace(/\.[^/.]+$/, "") + "_trimmed.wav";
            modalTrimmedFileToUpload = new File([wavBlob], newFileName, { type: 'audio/wav' });
            
            if (modalTrimPreviewAudio) {
                modalTrimPreviewAudio.src = URL.createObjectURL(modalTrimmedFileToUpload);
                modalTrimPreviewAudio.play();
            }
            if (btnModalTrimUpload) btnModalTrimUpload.disabled = false;
            showToast('Đã cắt 5s thành công! Hãy nghe thử và bấm "Tải Lên & Lưu Giọng".', 'success');
        });
    }

    async function uploadCloneFileModal(file) {
        const saveNameInput = document.getElementById('modal-clone-save-name');
        const saveGenderInput = document.getElementById('modal-clone-save-gender');
        const saveRegionInput = document.getElementById('modal-clone-save-region');
        const saveStyleInput = document.getElementById('modal-clone-save-style');

        const nameVal = (saveNameInput && saveNameInput.value.trim()) ? saveNameInput.value.trim() : (file.name.replace(/\.[^/.]+$/, "") || 'Giọng Clone');
        const genderVal = saveGenderInput ? saveGenderInput.value : '';
        const regionVal = saveRegionInput ? saveRegionInput.value : '';
        const styleVal = saveStyleInput ? saveStyleInput.value.trim() : '';

        const formData = new FormData();
        formData.append('file', file);
        formData.append('name', nameVal);
        formData.append('gender', genderVal);
        formData.append('region', regionVal);
        formData.append('style', styleVal);
        
        try {
            const response = await fetch('/api/clone/upload', {
                method: 'POST',
                body: formData
            });

            const data = await response.json();
            if (!response.ok) throw new Error(data.detail || 'Lỗi server');

            currentCloneId = data.clone_id;
            cloneIdDisplay.textContent = currentCloneId;
            cloneStatus.classList.remove('hidden');
            if (btnSynthClone) btnSynthClone.disabled = false;
            
            showToast(`Đã tạo & lưu thành công giọng "${nameVal}"!`, 'success');
            hideCreateVoiceModal();
            loadSavedVoices(data.clone_id);
            return true;
        } catch (error) {
            showToast('Lỗi: ' + error.message, 'error');
            return false;
        } finally {
            if (btnModalTrimUpload) {
                btnModalTrimUpload.disabled = false;
                btnModalTrimUpload.innerHTML = '<i class="fa-solid fa-cloud-arrow-up"></i> Tải Lên & Lưu Giọng';
            }
        }
    }

    async function loadSavedVoices(autoSelectId = null) {
        const selectEl = document.getElementById('clone-saved-voices-select');
        const customOptionsEl = document.getElementById('custom-clone-voice-options');
        if (!selectEl || !customOptionsEl) return;

        try {
            const res = await fetch('/api/clone/voices');
            if (!res.ok) return;
            const voices = await res.json();
            
            selectEl.innerHTML = '<option value="">-- Chọn giọng đã lưu sẵn --</option>';
            customOptionsEl.innerHTML = `
                <div class="custom-option selected" data-value="">
                    <div class="voice-title" style="font-size: 0.9rem; margin-bottom: 0;"><i class="fa-solid fa-microphone-slash text-muted"></i> -- Chọn giọng mẫu --</div>
                </div>
            `;

            let autoSelectOption = null;

            voices.forEach(v => {
                const opt = document.createElement('option');
                opt.value = v.id;
                opt.textContent = `${v.name || 'Giọng mẫu'} (${v.gender || ''} ${v.region || ''})`;
                selectEl.appendChild(opt);

                const optionDiv = document.createElement('div');
                optionDiv.className = 'custom-option';
                optionDiv.dataset.value = v.id;
                
                let badgesHtml = '';
                if (v.gender) {
                    const gClass = v.gender.toLowerCase() === 'nữ' ? 'gender-nu' : 'gender-nam';
                    badgesHtml += `<span class="badge ${gClass}">${v.gender}</span>`;
                }
                if (v.region) badgesHtml += `<span class="badge region">${v.region}</span>`;
                if (v.style) badgesHtml += `<span class="badge style">${v.style}</span>`;

                optionDiv.innerHTML = `
                    <div style="display: flex; justify-content: space-between; align-items: center;">
                        <div style="display: flex; align-items: center; gap: 8px; flex-wrap: wrap;">
                            <span class="voice-title" style="font-size: 0.95rem; margin-bottom: 0; color: #fff; font-weight: 600;">
                                <i class="fa-solid fa-microphone-lines text-primary"></i> ${v.name || 'Giọng mẫu'}
                            </span>
                            <div class="voice-badges" style="display: inline-flex; gap: 4px; align-items: center;">${badgesHtml}</div>
                        </div>
                        <button class="btn-delete-saved-voice" data-id="${v.id}" type="button" style="background: transparent; border: none; color: #ef4444; cursor: pointer; padding: 4px; font-size: 0.85rem;" title="Xóa giọng này"><i class="fa-solid fa-trash-can"></i></button>
                    </div>
                `;
                customOptionsEl.appendChild(optionDiv);

                if (autoSelectId && v.id === autoSelectId) {
                    autoSelectOption = optionDiv;
                }
            });

            bindCustomCloneSelectEvents();

            if (autoSelectOption) {
                autoSelectOption.click();
            }
        } catch (e) {
            console.error('Lỗi nạp danh sách giọng đã lưu:', e);
        }
    }

    function bindCustomCloneSelectEvents() {
        const wrapper = document.getElementById('custom-clone-voice-select');
        const trigger = wrapper ? wrapper.querySelector('.custom-select-trigger') : null;
        const optionsContainer = document.getElementById('custom-clone-voice-options');
        const triggerInfo = document.getElementById('selected-clone-voice-info');
        const realSelect = document.getElementById('clone-saved-voices-select');

        if (!wrapper || !trigger || !optionsContainer) return;

        trigger.onclick = (e) => {
            e.stopPropagation();
            document.querySelectorAll('.custom-select-wrapper').forEach(w => {
                if (w !== wrapper) w.classList.remove('open');
            });
            wrapper.classList.toggle('open');
        };

        optionsContainer.querySelectorAll('.custom-option').forEach(opt => {
            opt.onclick = (e) => {
                if (e.target.closest('.btn-delete-saved-voice')) return;
                
                const val = opt.dataset.value;
                optionsContainer.querySelectorAll('.custom-option').forEach(o => o.classList.remove('selected'));
                opt.classList.add('selected');
                
                if (realSelect) realSelect.value = val;
                
                if (val) {
                    currentCloneId = val;
                    cloneIdDisplay.textContent = currentCloneId;
                    cloneStatus.classList.remove('hidden');
                    cloneGenerateSection.classList.add('enabled');
                    if (btnSynthClone) btnSynthClone.disabled = false;
                    
                    const nameText = opt.querySelector('.voice-title')?.innerText || 'Giọng đã chọn';
                    const badgesHtml = opt.querySelector('.voice-badges')?.innerHTML || '';

                    triggerInfo.innerHTML = `
                        <div style="display: flex; align-items: center; gap: 8px; flex-wrap: wrap;">
                            <i class="fa-solid fa-microphone text-primary"></i>
                            <strong style="color: #c084fc;">${nameText}</strong>
                            <div class="voice-badges" style="display: inline-flex; gap: 4px;">${badgesHtml}</div>
                        </div>
                    `;
                    showToast('Đã chọn giọng mẫu: ' + nameText, 'success');
                } else {
                    currentCloneId = null;
                    triggerInfo.innerHTML = `<i class="fa-solid fa-microphone text-primary"></i> <span>-- Chọn giọng đã lưu sẵn --</span>`;
                }

                wrapper.classList.remove('open');
            };
        });

        optionsContainer.querySelectorAll('.btn-delete-saved-voice').forEach(btn => {
            btn.onclick = async (e) => {
                e.stopPropagation();
                const idToDelete = btn.dataset.id;
                if (!idToDelete) return;
                if (confirm('Bạn có chắc muốn xóa giọng mẫu này không?')) {
                    try {
                        const res = await fetch(`/api/clone/voices/${idToDelete}`, { method: 'DELETE' });
                        if (res.ok) {
                            showToast('Đã xóa giọng mẫu!', 'success');
                            if (currentCloneId === idToDelete) {
                                currentCloneId = null;
                                triggerInfo.innerHTML = `<i class="fa-solid fa-microphone text-primary"></i> <span>-- Chọn giọng đã lưu sẵn --</span>`;
                            }
                            loadSavedVoices();
                        } else {
                            showToast('Lỗi xóa giọng mẫu', 'error');
                        }
                    } catch (err) {
                        showToast('Lỗi: ' + err.message, 'error');
                    }
                }
            };
        });
    }

    loadSavedVoices();

// Update Clone Synth logic to use Dropdown

//===========================================
// HISTORY & ADMIN UI LOGIC
//===========================================
const dashboardModal = document.getElementById('dashboard-modal');
const dashboardTitle = document.getElementById('dashboard-title');
const dashboardContent = document.getElementById('dashboard-content');
const btnCloseDashboard = document.getElementById('btn-close-dashboard');
const btnBackToAdmin = document.getElementById('btn-back-to-admin');

document.getElementById('btn-show-history')?.addEventListener('click', () => {
    dashboardModal.classList.remove('hidden');
    dashboardTitle.innerText = "Lịch sử tạo âm thanh của bạn";
    btnBackToAdmin.classList.add('hidden');
    loadHistory('me');
});

document.getElementById('btn-show-admin')?.addEventListener('click', () => {
    dashboardModal.classList.remove('hidden');
    dashboardTitle.innerText = "Quản trị người dùng";
    btnBackToAdmin.classList.add('hidden');
    loadAdminUsers();
});

btnCloseDashboard?.addEventListener('click', () => {
    dashboardModal.classList.add('hidden');
});

btnBackToAdmin?.addEventListener('click', () => {
    dashboardTitle.innerText = "Quản trị người dùng";
    btnBackToAdmin.classList.add('hidden');
    loadAdminUsers();
});

async function loadAdminUsers() {
    dashboardContent.innerHTML = '<p style="color: white; text-align: center;">Đang tải...</p>';
    try {
        const res = await fetch('/api/admin/users');
        if (!res.ok) {
            dashboardContent.innerHTML = '<p style="color: red; text-align: center;">Lỗi tải dữ liệu</p>';
            return;
        }
        const users = await res.json();
        let html = `
            <table class="glass-table">
                <tr>
                    <th>Tên Đăng Nhập</th>
                    <th>Quyền</th>
                    <th>Trạng thái</th>
                    <th>Hành động</th>
                </tr>
        `;
        users.forEach(u => {
            html += `
                <tr>
                    <td><div class="line-clamp-3">${u.username}</div></td>
                    <td>${u.role}</td>
                    <td>${u.is_approved ? '<span class="badge badge-success"><i class="fa-solid fa-check"></i> Đã duyệt</span>' : '<span class="badge badge-warning"><i class="fa-solid fa-hourglass-half"></i> Chờ duyệt</span>'}</td>
                    <td style="display: flex; gap: 8px;">
                        ${!u.is_approved ? `<button onclick="approveUser('${u.id}')" class="action-btn approve"><i class="fa-solid fa-check-circle"></i> Duyệt</button>` : ''}
                        <button onclick="viewUserHistory('${u.id}', '${u.username}')" class="action-btn history"><i class="fa-solid fa-history"></i> Lịch sử</button>
                    </td>
                </tr>
            `;
        });
        html += '</table>';
        dashboardContent.innerHTML = html;
    } catch(e) {
        dashboardContent.innerHTML = '<p style="color: red; text-align: center;">Lỗi tải dữ liệu</p>';
    }
}

window.approveUser = async function(id) {
    try {
        await fetch(`/api/admin/users/${id}/approve`, { method: 'POST' });
        loadAdminUsers();
    } catch(e) {}
}

window.viewUserHistory = function(id, username) {
    dashboardTitle.innerText = "Lịch sử của: " + username;
    btnBackToAdmin.classList.remove('hidden');
    loadHistory(id);
}

async function loadHistory(userId) {
    dashboardContent.innerHTML = '<p style="color: white; text-align: center;">Đang tải...</p>';
    try {
        let url = userId === 'me' ? '/api/history' : `/api/admin/users/${userId}/history`;
        const res = await fetch(url);
        if (!res.ok) {
            if (res.status === 401) {
                dashboardContent.innerHTML = `
                    <div style="text-align: center; padding: 40px 20px; color: #f59e0b;">
                        <i class="fa-solid fa-lock" style="font-size: 3rem; margin-bottom: 15px; opacity: 0.8;"></i>
                        <p>Phiên đăng nhập hết hạn, vui lòng tải lại trang.</p>
                    </div>`;
            } else {
                dashboardContent.innerHTML = `
                    <div style="text-align: center; padding: 40px 20px; color: #ef4444;">
                        <i class="fa-solid fa-server" style="font-size: 3rem; margin-bottom: 15px; opacity: 0.8;"></i>
                        <p>Lỗi máy chủ nội bộ. Vui lòng thử lại sau.</p>
                    </div>`;
            }
            return;
        }
        const history = await res.json();
        
        if (history.length === 0) {
            dashboardContent.innerHTML = '<div style="text-align: center; padding: 40px 20px; color: #94a3b8;"><i class="fa-solid fa-box-open" style="font-size: 3rem; margin-bottom: 15px; opacity: 0.5;"></i><p>Chưa có dữ liệu lịch sử nào.</p></div>';
            return;
        }
        
        let html = `
            <table class="glass-table" style="font-size: 0.85rem;">
                <tr>
                    <th style="padding: 8px 10px;">Thời gian</th>
                    <th style="padding: 8px 10px; min-width: 120px;">Engine</th>
                    <th style="padding: 8px 10px;">Giọng đọc</th>
                    <th style="padding: 8px 10px; width: 30%;">Nội dung</th>
                    <th style="padding: 8px 10px;">Tiến trình</th>
                    <th style="padding: 8px 10px;">Hành động</th>
                </tr>
        `;
        history.forEach(h => {
            let engineBadge = `<span class="badge" style="background: rgba(255,255,255,0.1); color: #e2e8f0; border: 1px solid rgba(255,255,255,0.2); padding: 3px 6px;">${h.engine}</span>`;
            if (h.engine === 'standard') engineBadge = `<span class="badge" style="background: rgba(59,130,246,0.2); color: #60a5fa; border: 1px solid rgba(59,130,246,0.3); padding: 3px 6px;"><i class="fa-solid fa-wave-square"></i> Standard</span>`;
            if (h.engine === 'fasttts') engineBadge = `<span class="badge" style="background: rgba(16,185,129,0.2); color: #34d399; border: 1px solid rgba(16,185,129,0.3); padding: 3px 6px;"><i class="fa-solid fa-bolt"></i> FastTTS</span>`;
            if (h.engine === 'clone') engineBadge = `<span class="badge" style="background: rgba(236,72,153,0.2); color: #f472b6; border: 1px solid rgba(236,72,153,0.3); padding: 3px 6px;"><i class="fa-solid fa-users-viewfinder"></i> Clone</span>`;
            
            let progressRaw = h.progress || '0/0';
            let pColor = '#10b981'; // Xanh lá
            let pParts = progressRaw.split('/');
            if (pParts.length === 2 && pParts[0] !== pParts[1]) {
                pColor = '#f59e0b'; // Vàng
            }
            
            html += `
                <tr>
                    <td style="padding: 8px 10px; font-weight: 500; color: #a5b4fc; max-width: 85px;" title="${h.time_ago}">
                        <div class="line-clamp-3">${h.time_ago}</div>
                    </td>
                    <td style="padding: 8px 10px;">${engineBadge}</td>
                    <td style="padding: 8px 10px; max-width: 130px;" title="${h.voice} (${h.speed}x)">
                        <div class="line-clamp-3">
                            <span style="font-weight: 500; color: #e2e8f0;">${h.voice}</span> 
                            <span style="font-size: 0.85em; opacity: 0.8; color: #a855f7;">(${h.speed}x)</span>
                        </div>
                    </td>
                    <td style="padding: 8px 10px; font-style: italic; color: #cbd5e1;" title="${h.text.replace(/"/g, '&quot;')}">
                        <div class="line-clamp-3">${h.text}</div>
                    </td>
                    <td style="padding: 8px 10px; font-weight: 600; color: ${pColor};">${progressRaw}</td>
                    <td style="padding: 8px 10px; text-align: center;">
                        <button onclick="reloadJob('${h.job_id}')" class="action-btn reload" style="padding: 6px 8px; font-size: 0.8rem; flex-direction: column; align-items: center; justify-content: center; gap: 4px; width: 100%;">
                            <i class="fa-solid fa-rotate-right" style="font-size: 1.1em;"></i> 
                            <span>Nạp lại</span>
                        </button>
                    </td>
                </tr>
            `;
        });
        html += '</table>';
        dashboardContent.innerHTML = html;
    } catch(e) {
        dashboardContent.innerHTML = '<p style="color: red; text-align: center;">Lỗi tải dữ liệu</p>';
    }
}

window.reloadJob = async function(jobId) {
    try {
        const res = await fetch(`/api/history/${jobId}`);
        if (!res.ok) return showToast('Lỗi tải Job', 'error');
        const job = await res.json();
        
        // 1. Đóng modal
        dashboardModal.classList.add('hidden');
        
        // 2. Điền lại Text
        mainText.value = job.text;
        mainText.readOnly = true;
        mainText.style.opacity = 0.7;
        mainText.style.cursor = 'not-allowed';
        const customReplace = document.getElementById('custom-replace');
        if (customReplace) {
            customReplace.readOnly = true;
            customReplace.style.opacity = 0.7;
            customReplace.style.cursor = 'not-allowed';
        }
        const btnCustomReplace = document.getElementById('btn-custom-replace');
        if (btnCustomReplace) {
            btnCustomReplace.disabled = true;
            btnCustomReplace.style.opacity = '0.5';
            btnCustomReplace.style.cursor = 'not-allowed';
        }
        disableInactiveTabs(true);
        if (typeof processMainTextChunks === 'function') processMainTextChunks();
        
        currentJobId = job.job_id;
        
        // 3. Chọn Tab Model
        const tabBtn = document.querySelector(`.tab-btn[data-tab="${job.engine}"]`);
        if (tabBtn) tabBtn.click();
        
        // 4. Khôi phục setting
        if (job.engine === 'standard' && document.getElementById('std-voice')) {
            document.getElementById('std-voice').value = job.voice;
        } else if (job.engine === 'fasttts') {
            const radio = document.querySelector(`input[name="fasttts-voice-radio"][value="${job.voice}"]`);
            if (radio) radio.checked = true;
        } else if (job.engine === 'clone' && cloneSelect) {
            cloneSelect.value = job.voice;
        }
        if (job.engine === 'standard' && document.getElementById('std-speed')) {
            document.getElementById('std-speed').value = job.speed;
            document.getElementById('std-speed-val').innerText = job.speed + 'x';
        } else if (job.engine === 'fasttts' && document.getElementById('fasttts-speed')) {
            document.getElementById('fasttts-speed').value = job.speed;
            document.getElementById('fasttts-speed-val').innerText = job.speed + 'x';
        }
        
        // 5. Hiển thị lại các chunk đã chạy
        if (job.chunks && job.chunks.length > 0) {
            streamingState = {
                active: false,
                cancelled: true,
                engine: job.engine,
                params: { speed: job.speed, voice: job.voice },
                chunks: [],
                currentPlayIndex: -1,
                currentGenIndex: job.chunks.length,
                totalRetries: 0
            };
            
            toggleTextPanel(false);
            toggleSettingsPanel(false);
            if (streamingPanel) streamingPanel.classList.remove('hidden');
            
            if (streamingChunkList) streamingChunkList.innerHTML = '';
            const fragment = document.createDocumentFragment();
            
            const chunkTexts = splitTextIntoChunks(job.text, 1000, 2000);
            const dbChunksMap = {};
            job.chunks.forEach(c => dbChunksMap[c.chunk_index] = c);
            
            chunkTexts.forEach((ctext, i) => {
                const btn = document.createElement('button');
                btn.className = 'btn secondary-btn';
                
                const c = dbChunksMap[i];
                let bgColor, textColor, borderColor, status, blobUrl;
                
                if (c) {
                    status = c.status === 'error' ? 'error' : 'ready';
                    blobUrl = c.audio_path ? `/${c.audio_path}` : null;
                } else {
                    status = 'pending';
                    blobUrl = null;
                }
                
                btn.style.cssText = `padding: 6px 14px; font-size: 0.85em; white-space: nowrap; flex-shrink: 0; border-radius: 6px; transition: all 0.2s ease; cursor: pointer;`;
                let prevStr = ctext.substring(0, 25).replace(/\n/g, ' ').replace(/\s+/g, ' ').trim();
                btn.innerText = `Đoạn ${i + 1}: ${prevStr}...`;
                btn.title = `Bấm để nghe Đoạn ${i + 1}`;
                
                btn.onclick = () => {
                    jumpToChunk(i);
                };
                

                const chunkItem = {
                    index: i,
                    text: ctext,
                    status: status,
                    blobUrl: blobUrl,
                    btnEl: btn
                };
                streamingState.chunks.push(chunkItem);
                updateChunkUI(i); // Đảm bảo màu sắc được áp dụng đúng chuẩn
                fragment.appendChild(btn);
            });
            if (streamingChunkList) streamingChunkList.appendChild(fragment);
            if (streamingProgressText) streamingProgressText.innerText = `${job.chunks.length} / ${chunkTexts.length}`;
            
            if (btnCancelStreaming) btnCancelStreaming.classList.add('hidden');
            if (btnResumeStreaming) {
                // Kiểm tra xem có chunk nào pending không, nếu có thì hiện nút Tiếp tục
                if (chunkTexts.length > job.chunks.length) {
                    btnResumeStreaming.classList.remove('hidden');
                } else {
                    btnResumeStreaming.classList.add('hidden');
                }
                
                btnResumeStreaming.onclick = () => {
                    btnResumeStreaming.classList.add('hidden');
                    btnCancelStreaming.classList.remove('hidden');
                    streamingState.cancelled = false;
                    streamingState.active = true;
                    generateChunksLoop();
                };
            }
            if (btnDownloadCombinedMp3) {
                btnDownloadCombinedMp3.classList.remove('hidden');
                btnDownloadCombinedMp3.onclick = async () => {
                    const validChunks = streamingState.chunks.filter(c => c.blobUrl);
                    if (validChunks.length === 0) return showToast('Chưa có dữ liệu Audio!', 'error');
                    
                    showToast('Đang tạo file gộp...', 'info');
                    let allBlobs = [];
                    for (let c of validChunks) {
                        try {
                            const r = await fetch(c.blobUrl);
                            if (r.ok) {
                                const b = await r.blob();
                                allBlobs.push(b);
                            }
                        } catch(e) { console.error(e); }
                    }
                    if (allBlobs.length === 0) return showToast('Lỗi tải Audio!', 'error');
                    
                    const finalBlob = new Blob(allBlobs, { type: 'audio/mpeg' });
                    const url = URL.createObjectURL(finalBlob);
                    const a = document.createElement('a');
                    a.href = url;
                    a.download = `VieneuTTS_History_${Date.now()}.mp3`;
                    a.click();
                    URL.revokeObjectURL(url);
                    showToast('Đã tải xuống file gộp!', 'success');
                };
            }
            showToast('Đã nạp lại trạng thái công việc!', 'success');
        } else {
            showToast('Job này chưa được lưu lịch sử đầy đủ!', 'error');
        }
    } catch(e) {
        console.error(e);
        showToast('Lỗi khi Nạp lại Job', 'error');
    }
}

});
