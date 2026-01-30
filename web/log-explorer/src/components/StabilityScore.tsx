import { memo } from 'react';
import { Icon } from './Icon';

interface StabilityScoreProps {
  score: number;
  dropRate: number;
  reconnectRate: number;
  protocolErrorRate: number;
  loading?: boolean;
}

function getScoreColor(score: number): string {
  if (score >= 90) return '#4ade80';
  if (score >= 70) return '#fbbf24';
  if (score >= 50) return '#fb923c';
  return '#f87171';
}

function getScoreLabel(score: number): string {
  if (score >= 90) return 'Excellent';
  if (score >= 70) return 'Good';
  if (score >= 50) return 'Fair';
  return 'Poor';
}

function StabilityScoreComponent({ score, dropRate, reconnectRate, protocolErrorRate, loading }: StabilityScoreProps) {
  const safeScore = Number.isFinite(score) ? Math.min(100, Math.max(0, score)) : 0;
  
  if (loading) {
    return (
      <div className="stability-score-container" role="region" aria-label="Connection Stability Score">
        <div className="stability-score-loading">
          <div className="spinner" aria-hidden="true" />
          <span>Loading stability data...</span>
        </div>
      </div>
    );
  }

  const color = getScoreColor(safeScore);
  const label = getScoreLabel(safeScore);
  const circumference = 2 * Math.PI * 45;
  const strokeDashoffset = circumference - (safeScore / 100) * circumference;

  return (
    <div className="stability-score-container" role="region" aria-label="Connection Stability Score">
      <div className="stability-score-header">
        <h3><Icon name="shield" size="lg" aria-hidden={true} /> Stability Score</h3>
      </div>
      <div className="stability-score-content">
        <div className="stability-gauge">
          <svg viewBox="0 0 100 100" className="stability-ring">
            <circle
              cx="50"
              cy="50"
              r="45"
              fill="none"
              stroke="var(--border-subtle)"
              strokeWidth="8"
            />
            <circle
              cx="50"
              cy="50"
              r="45"
              fill="none"
              stroke={color}
              strokeWidth="8"
              strokeLinecap="round"
              strokeDasharray={circumference}
              strokeDashoffset={strokeDashoffset}
              transform="rotate(-90 50 50)"
              style={{ transition: 'stroke-dashoffset 0.5s ease-in-out' }}
            />
          </svg>
          <div className="stability-score-value">
            <span className="score-number" style={{ color }}>{safeScore.toFixed(0)}</span>
            <span className="score-label">{label}</span>
          </div>
        </div>
        <div className="stability-breakdown">
          <div className="breakdown-item">
            <span className="breakdown-label">Drop Rate</span>
            <span className="breakdown-value" style={{ color: dropRate > 0.1 ? '#f87171' : '#4ade80' }}>
              {(dropRate * 100).toFixed(1)}%
            </span>
          </div>
          <div className="breakdown-item">
            <span className="breakdown-label">Reconnect Rate</span>
            <span className="breakdown-value" style={{ color: reconnectRate > 0.2 ? '#fbbf24' : '#4ade80' }}>
              {(reconnectRate * 100).toFixed(1)}%
            </span>
          </div>
          <div className="breakdown-item">
            <span className="breakdown-label">Protocol Errors</span>
            <span className="breakdown-value" style={{ color: protocolErrorRate > 5 ? '#f87171' : '#4ade80' }}>
              {protocolErrorRate.toFixed(1)}‰
            </span>
          </div>
        </div>
      </div>
    </div>
  );
}

export const StabilityScore = memo(StabilityScoreComponent);
