import { type FormEvent, useState } from 'react';
import { Link } from 'react-router-dom';
import { Icon } from '@/components/Icon/Icon';
import { useApi } from '@/hooks/useApi';
import { api } from '@/lib/api';
import type { GroupSummary, OwnedClass } from '@/types/api';

/**
 * The class additions to the Study groups page: the classes you teach, with a link to each
 * dashboard, and a form to create one. A class is a study group whose owner — the teacher — also sees
 * members' per-topic mastery, misconceptions and reasoning, so the form says so before it is made.
 */
export function ClassCards() {
    const classes = useApi<{ classes: OwnedClass[] }>(() => api.get<{ classes: OwnedClass[] }>('/classes'));
    const [name, setName] = useState('');
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState<string | null>(null);

    async function create(event: FormEvent) {
        event.preventDefault();
        if (busy || name.trim().length < 2) return;
        setError(null);
        setBusy(true);
        try {
            await api.post<{ group: GroupSummary }>('/groups', { name: name.trim(), isClass: true });
            setName('');
            classes.reload();
        } catch (cause) {
            setError(cause instanceof Error ? cause.message : 'Could not create the class.');
        } finally {
            setBusy(false);
        }
    }

    const mine = classes.data?.classes ?? [];

    return (
        <>
            {mine.length > 0 && (
                <div className="card stack" style={{ gap: 8 }}>
                    <span className="label">Classes you teach</span>
                    {mine.map((c) => (
                        <Link key={c.id} to={`/classes/${c.id}`} className="row-item">
                            <span className="tile tile--blue" aria-hidden="true">
                                <Icon name="chart" />
                            </span>
                            <span className="row-item__body">
                                <strong>{c.name}</strong>
                                <div className="faint">
                                    Code <span className="mono">{c.inviteCode}</span> · {Math.max(0, c.memberCount - 1)}{' '}
                                    {c.memberCount - 1 === 1 ? 'student' : 'students'} · Open class dashboard
                                </div>
                            </span>
                            <Icon name="chevron" className="row-item__chev" />
                        </Link>
                    ))}
                </div>
            )}

            <form className="card stack" onSubmit={create}>
                <span className="label">Create a class</span>
                {error && <div className="banner banner--error">{error}</div>}
                <div className="field">
                    <label htmlFor="className">Class name</label>
                    <div className="inputwrap">
                        <span className="inputwrap__icon">
                            <Icon name="users" size={16} />
                        </span>
                        <input
                            id="className"
                            value={name}
                            maxLength={60}
                            placeholder="Class 10-B Physics"
                            disabled={busy}
                            onChange={(e) => setName(e.target.value)}
                        />
                    </div>
                    <span className="faint">
                        For teachers. Students join with the code like any group, and you get a class
                        dashboard: where the class is stuck, the misconceptions behind it and a re-teach
                        plan. Unlike a study group, you will see each student's per-topic mastery,
                        misconceptions and the reasoning they write.
                    </span>
                </div>
                <button type="submit" className="secondary wide" disabled={busy || name.trim().length < 2}>
                    {busy ? 'Working…' : 'Create class'}
                </button>
            </form>
        </>
    );
}
