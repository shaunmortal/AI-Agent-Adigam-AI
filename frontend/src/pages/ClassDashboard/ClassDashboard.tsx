import { useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { Icon } from '@/components/Icon/Icon';
import { Failed } from '@/components/Loading/States';
import { useApi } from '@/hooks/useApi';
import { api } from '@/lib/api';
import type { ClassInsights, ReteachPlan } from '@/types/api';

/**
 * The teacher's view of a class: where the whole class is stuck, the wrong ideas behind it in the
 * students' own words, everyone's mastery per topic, and a re-teach plan for the next lesson.
 *
 * Only the class's owner can load it; the server answers 403 to anybody else. Mastery is never shown
 * by colour alone — every bar and cell carries its number.
 */

const GAPS_SHOWN = 8;

function pct(score: number) {
    return `${Math.round(score)}%`;
}

export default function ClassDashboard() {
    const { id = '' } = useParams();
    const data = useApi<ClassInsights>(
        () => api.get<ClassInsights>(`/groups/${id}/insights`),
        [id]
    );

    const [plan, setPlan] = useState<ReteachPlan | null>(null);
    const [planning, setPlanning] = useState(false);
    const [planError, setPlanError] = useState<string | null>(null);
    const [showAllGaps, setShowAllGaps] = useState(false);

    async function makePlan() {
        setPlanError(null);
        setPlanning(true);
        try {
            const result = await api.post<{ plan: ReteachPlan }>(`/groups/${id}/insights/plan`, {});
            setPlan(result.plan);
        } catch (cause) {
            setPlanError(cause instanceof Error ? cause.message : 'Could not write a plan.');
        } finally {
            setPlanning(false);
        }
    }

    if (data.loading) {
        return (
            <div className="stack">
                <div className="skeleton" style={{ height: 40, width: '40%' }} />
                <div className="skeleton" style={{ height: 120 }} />
                <div className="skeleton" style={{ height: 260 }} />
            </div>
        );
    }
    if (data.error || !data.data) return <Failed error={data.error} onRetry={data.reload} />;

    const { class: cls, insights } = data.data;
    const gaps = showAllGaps ? insights.gaps : insights.gaps.slice(0, GAPS_SHOWN);
    const biggest = insights.gaps[0];

    return (
        <div className="stack">
            <div className="spread" style={{ alignItems: 'flex-start', gap: 16, flexWrap: 'wrap' }}>
                <div style={{ minWidth: 0 }}>
                    <Link to="/groups" className="faint">
                        ← Study groups
                    </Link>
                    <span className="label" style={{ display: 'block', marginTop: 8 }}>
                        Class dashboard · code <span className="mono">{cls.inviteCode}</span>
                    </span>
                    <h1 style={{ margin: '4px 0 0' }}>{cls.name}</h1>
                </div>
                <button type="button" className="secondary" onClick={data.reload}>
                    Refresh
                </button>
            </div>

            <div className="class-kpis">
                <div className="card">
                    <span className="label">Students</span>
                    <div className="big">{insights.totals.students}</div>
                </div>
                <div className="card">
                    <span className="label">Answers recorded</span>
                    <div className="big">{insights.totals.answers}</div>
                </div>
                <div className="card">
                    <span className="label">Active today</span>
                    <div className="big">{insights.totals.activeToday}</div>
                </div>
                <div className="card">
                    <span className="label">Biggest gap</span>
                    <div className="big">{biggest ? pct(biggest.avgMastery) : '—'}</div>
                    <div className="faint">{biggest ? biggest.topicName : 'No answers yet'}</div>
                </div>
            </div>

            {insights.totals.students === 0 && (
                <div className="banner banner--note">
                    No students yet. Share the code{' '}
                    <strong className="mono">{cls.inviteCode}</strong> with your class — they join
                    from Study groups.
                </div>
            )}

            <div className="class-split">
                <section className="card stack" aria-labelledby="gaps-title" style={{ gap: 14 }}>
                    <div>
                        <h2 id="gaps-title" className="label">
                            Class-wide gaps
                        </h2>
                        <p className="faint" style={{ margin: '4px 0 0' }}>
                            Ranked by how weak the class is and how many later topics each one
                            blocks.
                        </p>
                    </div>

                    {gaps.length === 0 && (
                        <p className="faint">Nothing to rank until students answer.</p>
                    )}

                    <ol
                        className="stack"
                        style={{ gap: 14, margin: 0, paddingLeft: 0, listStyle: 'none' }}
                    >
                        {gaps.map((gap, i) => (
                            <li key={gap.topicId} className="stack" style={{ gap: 6 }}>
                                <div className="spread" style={{ gap: 8 }}>
                                    <strong>
                                        <span className="faint mono">{i + 1}.</span> {gap.topicName}
                                    </strong>
                                    <span className="mono">{pct(gap.avgMastery)}</span>
                                </div>
                                <div
                                    className="meter"
                                    role="progressbar"
                                    aria-label={`${gap.topicName}: class average mastery`}
                                    aria-valuemin={0}
                                    aria-valuemax={100}
                                    aria-valuenow={Math.round(gap.avgMastery)}
                                >
                                    <div style={{ width: `${gap.avgMastery}%` }} />
                                </div>
                                <span className="faint">
                                    {gap.belowHalf} of {gap.studentsSeen} students below 50%
                                    {gap.downstream > 0 &&
                                        ` · blocks ${gap.downstream} later ${gap.downstream === 1 ? 'topic' : 'topics'}`}
                                </span>
                            </li>
                        ))}
                    </ol>

                    {insights.gaps.length > GAPS_SHOWN && (
                        <button
                            type="button"
                            className="secondary"
                            onClick={() => setShowAllGaps((v) => !v)}
                        >
                            {showAllGaps ? 'Show fewer' : `Show all ${insights.gaps.length}`}
                        </button>
                    )}
                </section>

                <section className="card stack" aria-labelledby="mc-title" style={{ gap: 12 }}>
                    <div>
                        <h2 id="mc-title" className="label">
                            Misconceptions in the class
                        </h2>
                        <p className="faint" style={{ margin: '4px 0 0' }}>
                            The wrong ideas behind wrong answers, with students' own reasoning.
                        </p>
                    </div>

                    {insights.misconceptions.length === 0 && (
                        <p className="faint">None spotted yet.</p>
                    )}

                    {insights.misconceptions.map((m) => (
                        <div
                            key={`${m.topicId}-${m.label}`}
                            className="card stack"
                            style={{ gap: 6 }}
                        >
                            <div className="spread" style={{ gap: 8, alignItems: 'flex-start' }}>
                                <strong>{m.label}</strong>
                                <span
                                    className="pill mono"
                                    style={{ padding: '2px 10px', whiteSpace: 'nowrap' }}
                                >
                                    {m.students} of {insights.totals.students}
                                </span>
                            </div>
                            <span className="faint">{m.topicName}</span>
                            {m.quotes.map((q) => (
                                <blockquote key={q} className="class-quote">
                                    “{q}”
                                </blockquote>
                            ))}
                        </div>
                    ))}
                </section>
            </div>

            <section
                className="card card--accent stack"
                aria-labelledby="plan-title"
                style={{ gap: 12 }}
            >
                <div className="spread" style={{ gap: 12, flexWrap: 'wrap' }}>
                    <div style={{ minWidth: 0 }}>
                        <h2 id="plan-title" className="label">
                            Re-teach plan for the next lesson
                        </h2>
                        <p className="faint" style={{ margin: '4px 0 0' }}>
                            AI reads the gaps and misconceptions above and drafts a 35-minute lesson
                            with student groups.
                        </p>
                    </div>
                    <button
                        type="button"
                        className="primary"
                        disabled={planning || insights.gaps.length === 0}
                        onClick={() => void makePlan()}
                    >
                        <Icon name="sparkles" size={16} />{' '}
                        {planning
                            ? 'Writing the plan…'
                            : plan
                              ? 'Write it again'
                              : 'Generate re-teach plan'}
                    </button>
                </div>

                {planError && <div className="banner banner--error">{planError}</div>}

                {plan && (
                    <div className="stack" style={{ gap: 12 }}>
                        <p style={{ margin: 0 }}>
                            <strong>{plan.headline}</strong>
                        </p>
                        <ol className="stack" style={{ gap: 10, margin: 0, paddingLeft: '1.2em' }}>
                            {plan.steps.map((step, i) => (
                                <li key={i}>
                                    <strong>{step.topic}</strong>{' '}
                                    <span className="faint">· {step.minutes} min</span>
                                    <div>{step.activity}</div>
                                    {step.addresses && step.addresses !== '-' && (
                                        <div className="faint">Fixes: {step.addresses}</div>
                                    )}
                                </li>
                            ))}
                        </ol>
                        <div className="class-plan-groups">
                            {plan.groups.map((group) => (
                                <div key={group.title} className="card stack" style={{ gap: 4 }}>
                                    <strong>{group.title}</strong>
                                    <span className="faint">{group.focus}</span>
                                    <span>{group.students.join(', ')}</span>
                                </div>
                            ))}
                        </div>
                    </div>
                )}
            </section>

            <section className="card stack" aria-labelledby="heat-title" style={{ gap: 10 }}>
                <div>
                    <h2 id="heat-title" className="label">
                        Every student × every topic
                    </h2>
                    <p className="faint" style={{ margin: '4px 0 0' }}>
                        Mastery per topic, weakest students first. A dash means no answers yet.
                    </p>
                </div>

                {insights.students.length === 0 || insights.topics.length === 0 ? (
                    <p className="faint">Nothing to show until students answer.</p>
                ) : (
                    <div
                        className="class-heat"
                        tabIndex={0}
                        aria-label="Mastery heatmap, scrolls sideways"
                    >
                        <table>
                            <thead>
                                <tr>
                                    <th scope="col" className="class-heat__name">
                                        Student
                                    </th>
                                    {insights.topics.map((t) => (
                                        <th
                                            key={t.id}
                                            scope="col"
                                            title={t.name}
                                            className="class-heat__topic"
                                        >
                                            <span>{t.name}</span>
                                        </th>
                                    ))}
                                    <th scope="col">Avg</th>
                                </tr>
                            </thead>
                            <tbody>
                                {insights.students.map((s) => (
                                    <tr key={s.key}>
                                        <th scope="row" className="class-heat__name">
                                            {s.name}
                                        </th>
                                        {insights.topics.map((t) => {
                                            const score = s.mastery[t.id];
                                            return (
                                                <td
                                                    key={t.id}
                                                    title={`${s.name} · ${t.name}: ${score === undefined ? 'no answers yet' : pct(score)}`}
                                                    style={
                                                        score === undefined
                                                            ? undefined
                                                            : {
                                                                  background: `color-mix(in srgb, var(--accent) ${Math.round(score)}%, transparent)`,
                                                                  color:
                                                                      score >= 55
                                                                          ? '#fff'
                                                                          : undefined,
                                                              }
                                                    }
                                                >
                                                    {score === undefined ? '–' : Math.round(score)}
                                                </td>
                                            );
                                        })}
                                        <td className="mono">
                                            <strong>{pct(s.avgMastery)}</strong>
                                        </td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>
                )}
            </section>
        </div>
    );
}
