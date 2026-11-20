// Pure browser-independent helpers, also exercised with node:test.
export function validName(name) { return typeof name === 'string' && name.length > 0 && name !== '.' && name !== '..' && !/[\/\0\r\n]/.test(name); }
export function parentPath(path) { const parts = path.split('/').filter(Boolean); parts.pop(); return '/' + parts.join('/'); }
export function joinPath(path, name) { return (path === '/' ? '' : path.replace(/\/$/, '')) + '/' + name; }
export function formatSize(bytes) { if (!Number.isFinite(bytes) || bytes < 0) return '—'; if (bytes < 1024) return bytes + ' B'; const units = ['KB','MB','GB','TB']; let n = bytes / 1024, i = 0; while (n >= 1024 && i < units.length - 1) { n /= 1024; i++; } return n.toLocaleString('zh-CN', {maximumFractionDigits:1}) + ' ' + units[i]; }
export function kindOf(name) { const ext = name.split('.').pop().toLowerCase(); if (/^(jpg|jpeg|png|gif|bmp|webp|ico)$/.test(ext)) return 'image'; if (/^(mp4|webm|mov|m4v)$/.test(ext)) return 'video'; if (/^(mp3|wav|flac|aac|ogg|m4a|opus)$/.test(ext)) return 'audio'; return 'file'; }
export function normalizeEntries(entries) { return (entries || []).filter(e => validName(e.Name)).map(e => ({name:e.Name, isDir:!!e.IsDir, size:e.Size || 0, date:e.ModTime || ''})); }
const collator = new Intl.Collator('zh-CN', {numeric:true, sensitivity:'base'});
export function visibleEntries(entries, search, sort) { const q = search.toLocaleLowerCase(); return entries.filter(e => e.name.toLocaleLowerCase().includes(q)).slice().sort((a,b) => { if (a.isDir !== b.isDir) return a.isDir ? -1 : 1; if (sort === 'size' && a.size !== b.size) return b.size - a.size; if (sort === 'date' && a.date !== b.date) return b.date.localeCompare(a.date); return collator.compare(a.name,b.name) * (sort === 'name-desc' ? -1 : 1); }); }
