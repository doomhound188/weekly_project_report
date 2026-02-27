// Weekly Report Generator - Frontend Application
(function () {
    'use strict';

    const $ = (sel) => document.querySelector(sel);
    let currentReport = null;

    // ── Init ────────────────────────────────────────────────────────
    document.addEventListener('DOMContentLoaded', () => {
        loadUser();
        loadMembers();
        setupDatePicker();
        setupForm();
        setupDownload();
        setupSchedule();
    });

    // ── Auth / User ─────────────────────────────────────────────────
    async function loadUser() {
        try {
            const resp = await fetch('/auth/user');
            const user = await resp.json();
            if (user.authenticated && user.name) {
                const bar = $('#user-bar');
                bar.style.display = 'flex';
                $('#user-name').textContent = 'Welcome, ' + user.name;
            }
        } catch (e) {
            // Auth not configured — user bar stays hidden
        }
    }

    // ── Members ─────────────────────────────────────────────────────
    async function loadMembers() {
        const select = $('#member_id');
        try {
            const resp = await fetch('/api/members');
            if (!resp.ok) {
                const err = await resp.json();
                throw new Error(err.error || 'Failed to load members');
            }
            const members = await resp.json();

            select.innerHTML = '<option value="">-- Select a team member --</option>';
            members.forEach((m) => {
                const opt = document.createElement('option');
                opt.value = m.identifier;
                opt.textContent = `${m.name} (${m.identifier})`;
                select.appendChild(opt);
            });
        } catch (e) {
            select.innerHTML = '<option value="">-- Error loading members --</option>';
            showAlert('error', '❌ ' + e.message);
        }
    }

    // ── Date Picker ─────────────────────────────────────────────────
    function setupDatePicker() {
        const input = $('#end_date');
        const today = new Date().toISOString().split('T')[0];
        input.value = today;
        input.max = today;
    }

    // ── Form Submission ─────────────────────────────────────────────
    function setupForm() {
        const form = $('#report-form');
        form.addEventListener('submit', async (e) => {
            e.preventDefault();

            const memberID = $('#member_id').value;
            if (!memberID) {
                showAlert('error', '❌ Please select a team member');
                return;
            }

            const payload = {
                member_id: memberID,
                days: parseInt($('#days').value, 10),
                end_date: $('#end_date').value,
                compare_projects: $('#compare_projects').checked,
                send_email: $('#send_email').checked,
                schedule_week: $('#schedule_week').checked,
            };

            // Show loading
            const btn = $('#generate-btn');
            btn.disabled = true;
            btn.textContent = '⏳ Generating...';
            showProgress(true);
            hideReport();
            clearAlerts();

            // Animate progress
            let progress = 0;
            const progressInterval = setInterval(() => {
                progress = Math.min(progress + Math.random() * 15, 90);
                setProgress(progress, getProgressMessage(progress));
            }, 800);

            try {
                const resp = await fetch('/api/generate', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(payload),
                });

                const result = await resp.json();

                clearInterval(progressInterval);
                setProgress(100, 'Done!');

                setTimeout(() => {
                    showProgress(false);

                    if (result.success) {
                        currentReport = result;

                        let reportText = result.report_body;
                        if (result.comparison_summary) {
                            reportText += '\n\n' + '='.repeat(50) + '\n';
                            reportText += 'PROJECT COMPARISON\n';
                            reportText += '='.repeat(50) + '\n';
                            reportText += result.comparison_summary;
                        }

                        // Display report
                        let successMsg = '✅ Report generated successfully!';
                        if (result.email_sent) successMsg += ' Email has been sent.';
                        showAlert('success', successMsg);

                        showReport({
                            text: reportText,
                            subject: result.report_subject,
                            aiProvider: result.ai_provider,
                            entryCount: result.entry_count,
                            projectCount: result.project_count,
                        });

                        // Auto-trigger schedule if checkbox was checked
                        if (payload.schedule_week) {
                            triggerSchedule(memberID, result.report_body);
                        }
                    } else {
                        showAlert('error', '❌ ' + (result.error || 'Unknown error'));
                    }
                }, 400);
            } catch (e) {
                clearInterval(progressInterval);
                showProgress(false);
                showAlert('error', '❌ Network error: ' + e.message);
            } finally {
                btn.disabled = false;
                btn.textContent = '🚀 Generate Report';
            }
        });
    }

    // ── Download ────────────────────────────────────────────────────
    function setupDownload() {
        $('#download-btn').addEventListener('click', () => {
            if (!currentReport) return;

            let text = currentReport.report_body;
            if (currentReport.comparison_summary) {
                text += '\n\n' + '='.repeat(50) + '\nPROJECT COMPARISON\n' + '='.repeat(50) + '\n' + currentReport.comparison_summary;
            }

            const blob = new Blob([text], { type: 'text/plain' });
            const url = URL.createObjectURL(blob);
            const a = document.createElement('a');
            const date = new Date().toISOString().split('T')[0];
            a.href = url;
            a.download = `weekly_report_${date}.txt`;
            a.click();
            URL.revokeObjectURL(url);
        });
    }

    // ── UI Helpers ──────────────────────────────────────────────────
    function showAlert(type, message) {
        const container = $('#alert-container');
        const alert = document.createElement('div');
        alert.className = `alert alert-${type}`;
        alert.textContent = message;
        container.appendChild(alert);

        // Auto-dismiss after 10s
        setTimeout(() => alert.remove(), 10000);
    }

    function clearAlerts() {
        $('#alert-container').innerHTML = '';
    }

    function showProgress(visible) {
        const container = $('#progress-container');
        if (visible) {
            container.classList.add('active');
        } else {
            container.classList.remove('active');
        }
    }

    function setProgress(percent, message) {
        $('#progress-fill').style.width = percent + '%';
        $('#progress-text').textContent = message;
    }

    function getProgressMessage(progress) {
        if (progress < 20) return '🔄 Fetching time entries from ConnectWise...';
        if (progress < 40) return '📋 Enriching with project details...';
        if (progress < 60) return '🤖 AI is generating your report...';
        if (progress < 80) return '📊 Organizing and formatting...';
        return '✨ Almost done...';
    }

    function showReport(data) {
        const container = $('#report-container');
        container.classList.add('active');

        // Meta badges
        const meta = $('#report-meta');
        meta.innerHTML = '';
        [data.aiProvider, `${data.entryCount} time entries`, `${data.projectCount} projects`].forEach((text) => {
            const badge = document.createElement('span');
            badge.className = 'meta-badge';
            badge.textContent = text;
            meta.appendChild(badge);
        });

        // Subject
        $('#report-info').innerHTML = `<strong>Subject:</strong> ${escapeHtml(data.subject)}`;

        // Body
        $('#report-content').textContent = data.text;

        // Scroll to report
        container.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }

    function hideReport() {
        $('#report-container').classList.remove('active');
    }

    function escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    // ── Schedule ────────────────────────────────────────────────────
    let currentSchedule = null;

    function setupSchedule() {
        $('#schedule-confirm-btn').addEventListener('click', async () => {
            if (!currentSchedule || !currentSchedule.blocks?.length) return;

            const btn = $('#schedule-confirm-btn');
            btn.disabled = true;
            btn.textContent = '⏳ Creating entries...';

            try {
                const resp = await fetch('/api/schedule/confirm', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        member_id: currentSchedule._member_id,
                        blocks: currentSchedule.blocks,
                    }),
                });

                const result = await resp.json();
                if (result.created_count > 0) {
                    showAlert('success', `✅ Created ${result.created_count} schedule entries in ConnectWise!`);
                }
                if (result.errors?.length > 0) {
                    showAlert('error', `⚠️ ${result.errors.length} entries failed: ${result.errors[0]}`);
                }
            } catch (e) {
                showAlert('error', '❌ Confirm error: ' + e.message);
            } finally {
                btn.disabled = false;
                btn.textContent = '✅ Confirm & Create in ConnectWise';
            }
        });
    }

    async function triggerSchedule(memberID, reportBody) {
        showAlert('info', '📅 Generating schedule proposals...');

        try {
            const resp = await fetch('/api/schedule', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    member_id: memberID,
                    report_body: reportBody,
                }),
            });

            const plan = await resp.json();

            if (plan.error) {
                showAlert('error', '❌ Schedule: ' + plan.error);
                return;
            }

            currentSchedule = plan;
            currentSchedule._member_id = memberID;
            renderSchedule(plan);
            showAlert('success', `📅 Proposed ${plan.blocks?.length || 0} blocks (${plan.total_hours?.toFixed(1) || 0} hrs)`);
        } catch (e) {
            showAlert('error', '❌ Schedule error: ' + e.message);
        }
    }

    function renderSchedule(plan) {
        const container = $('#schedule-container');
        const grid = $('#schedule-grid');
        const meta = $('#schedule-meta');
        const unsched = $('#schedule-unscheduled');

        grid.innerHTML = '';
        meta.innerHTML = '';
        unsched.innerHTML = '';
        unsched.classList.remove('active');

        // Meta badges
        const badges = [
            `Week of ${plan.week_starting}`,
            `${plan.total_hours?.toFixed(1) || 0} hrs scheduled`,
            `${plan.free_hours?.toFixed(1) || 0} hrs free`,
        ];
        badges.forEach((text) => {
            const badge = document.createElement('span');
            badge.className = 'meta-badge';
            badge.textContent = text;
            meta.appendChild(badge);
        });

        // Group blocks by day
        const blocks = plan.blocks || [];
        const days = {};
        blocks.forEach((b) => {
            const day = b.day || 'Unknown';
            if (!days[day]) days[day] = [];
            days[day].push(b);
        });

        // Render each day
        for (const [day, dayBlocks] of Object.entries(days)) {
            const dayDiv = document.createElement('div');
            dayDiv.className = 'schedule-day';
            dayDiv.innerHTML = `<div class="schedule-day-header">${escapeHtml(day)}</div>`;

            dayBlocks.forEach((b) => {
                const blockDiv = document.createElement('div');
                blockDiv.className = 'schedule-block';
                blockDiv.innerHTML = `
                    <span class="schedule-time">${escapeHtml(b.time_range)}</span>
                    <div class="schedule-task">
                        <div class="schedule-task-project">${escapeHtml(b.project)}</div>
                        <div class="schedule-task-step">→ ${escapeHtml(b.next_step)}</div>
                    </div>
                    <span class="schedule-hours">${b.hours}h</span>
                `;
                dayDiv.appendChild(blockDiv);
            });

            grid.appendChild(dayDiv);
        }

        // Unscheduled
        if (plan.unscheduled?.length > 0) {
            unsched.classList.add('active');
            unsched.innerHTML = `<strong>⚠️ Could not schedule:</strong><br>` +
                plan.unscheduled.map((t) => `• ${escapeHtml(t.project)} (${t.estimated_hours}h)`).join('<br>');
        }

        container.classList.add('active');
        container.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }
})();
