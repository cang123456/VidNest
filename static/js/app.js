// 简单的上传页交互：拖拽 + 进度条 + 多文件顺序上传
// 写成原生 JS 不依赖任何库，省得打包配置。
(function () {
    const zone = document.getElementById('dropZone');
    if (!zone) return;
    const fileInput = document.getElementById('fileInput');
    const listEl = document.getElementById('uploadList');

    // 点击区域 = 点隐藏的 input
    zone.addEventListener('click', () => fileInput.click());
    fileInput.addEventListener('change', () => {
        if (fileInput.files.length) handleFiles(fileInput.files);
        fileInput.value = ''; // 清空才能下次再选同一个文件
    });

    // 拖拽高亮
    ['dragenter', 'dragover'].forEach(ev => {
        zone.addEventListener(ev, e => {
            e.preventDefault();
            zone.classList.add('dragover');
        });
    });
    ['dragleave', 'drop'].forEach(ev => {
        zone.addEventListener(ev, e => {
            e.preventDefault();
            zone.classList.remove('dragover');
        });
    });
    zone.addEventListener('drop', e => {
        if (e.dataTransfer.files.length) handleFiles(e.dataTransfer.files);
    });

    function handleFiles(files) {
        for (let i = 0; i < files.length; i++) {
            uploadOne(files[i]);
        }
    }

    function uploadOne(file) {
        const item = document.createElement('div');
        item.className = 'upload-item';
        item.innerHTML = `
            <div class="name"></div>
            <div class="bar"><div class="fill" style="width:0"></div></div>
            <div class="percent">0%</div>
            <div class="status">上传中</div>
        `;
        item.querySelector('.name').textContent = file.name +
            `  (${formatSize(file.size)})`;
        listEl.appendChild(item);

        const fillEl = item.querySelector('.fill');
        const percentEl = item.querySelector('.percent');
        const statusEl = item.querySelector('.status');

        const xhr = new XMLHttpRequest();
        xhr.open('POST', '/api/upload');
        xhr.upload.onprogress = e => {
            if (!e.lengthComputable) return;
            const p = Math.round(e.loaded / e.total * 100);
            fillEl.style.width = p + '%';
            percentEl.textContent = p + '%';
        };
        xhr.onload = () => {
            fillEl.style.width = '100%';
            percentEl.textContent = '100%';
            try {
                const json = JSON.parse(xhr.responseText);
                if (json.ok) {
                    statusEl.textContent = '✓ 完成';
                    statusEl.className = 'status ok';
                    // 加一个跳转播放链接
                    const a = document.createElement('a');
                    a.href = '/play/' + json.id;
                    a.textContent = '去播放 →';
                    a.style.marginLeft = '10px';
                    statusEl.textContent = '';
                    statusEl.appendChild(document.createTextNode('✓ '));
                    statusEl.appendChild(a);
                } else {
                    statusEl.textContent = '✗ ' + (json.msg || '失败');
                    statusEl.className = 'status err';
                }
            } catch (e) {
                statusEl.textContent = '✗ 响应错误';
                statusEl.className = 'status err';
            }
        };
        xhr.onerror = () => {
            statusEl.textContent = '✗ 网络错误';
            statusEl.className = 'status err';
        };
        const fd = new FormData();
        fd.append('file', file);
        xhr.send(fd);
    }

    function formatSize(bytes) {
        if (bytes < 1024) return bytes + ' B';
        if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
        if (bytes < 1024 * 1024 * 1024) return (bytes / 1024 / 1024).toFixed(1) + ' MB';
        return (bytes / 1024 / 1024 / 1024).toFixed(2) + ' GB';
    }
})();

// 视频卡片的点击跳转（给首页和文件夹页用）
document.addEventListener('click', e => {
    const card = e.target.closest('[data-video-id]');
    if (card) {
        const id = card.getAttribute('data-video-id');
        if (id) location.href = '/play/' + id;
    }
});

// 顶部"重新扫描"按钮
document.addEventListener('click', e => {
    const btn = e.target.closest('[data-action=scan]');
    if (!btn) return;
    if (btn.disabled) return;
    btn.disabled = true;
    const old = btn.textContent;
    btn.textContent = '扫描中...';
    fetch('/api/scan', { method: 'POST' })
        .then(r => r.json())
        .then(() => {
            // 简单轮询 5 秒后刷新页面，用户能看到新增
            setTimeout(() => location.reload(), 1500);
        })
        .finally(() => {
            setTimeout(() => {
                btn.disabled = false;
                btn.textContent = old;
            }, 2000);
        });
});
