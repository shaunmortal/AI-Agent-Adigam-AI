import { Link } from 'react-router-dom';
import { Icon } from '@/components/Icon/Icon';
import { Empty, Failed, Loading } from '@/components/Loading/States';
import { useApi } from '@/hooks/useApi';
import { api } from '@/lib/api';
import { CourseMapView } from '@/pages/Sources/Sources';
import type { CourseMap, Goal } from '@/types/api';

/** The concept graph of the syllabus behind the learner's goal, coloured by their mastery. */
export default function ConceptMap() {
    const goal = useApi<{ goal: Goal | null }>(() => api.get<{ goal: Goal | null }>('/goals'));
    const curriculumId = goal.data?.goal?.curriculumId ?? null;
    const map = useApi<CourseMap | null>(
        () => (curriculumId ? api.get<CourseMap>(`/sources/map/${curriculumId}`) : Promise.resolve(null)),
        [curriculumId]
    );

    return (
        <div className="stack">
            <div className="row" style={{ gap: 14, alignItems: 'flex-start' }}>
                <span className="hero-mark" aria-hidden="true">
                    <Icon name="grid" size={40} />
                </span>
                <div style={{ minWidth: 0 }}>
                    <span className="label">Concept map</span>
                    <h1 style={{ marginBottom: 0 }}>How your syllabus fits together</h1>
                    <p className="muted" style={{ margin: '6px 0 0' }}>
                        Every concept, what it builds on, and how sure the tutor is that you have it.
                    </p>
                </div>
            </div>

            {(goal.loading || map.loading) && <Loading label="Drawing your concept map…" />}
            {goal.error ? <Failed error={goal.error} onRetry={goal.reload} /> : null}
            {map.error ? <Failed error={map.error} onRetry={map.reload} /> : null}
            {goal.data && !curriculumId && (
                <Empty title="No syllabus yet" action={<Link className="primary" to="/goals">Set a goal</Link>}>
                    Choose what you are studying (for example, Class 10 Physics) and its concept map appears here.
                </Empty>
            )}
            {map.data && <CourseMapView map={map.data} />}
        </div>
    );
}
