import { memo } from 'react';

interface SkeletonProps {
  width?: string | number;
  height?: string | number;
  variant?: 'text' | 'rect' | 'circle';
  className?: string;
}

function SkeletonComponent({
  width,
  height,
  variant = 'text',
  className = '',
}: SkeletonProps) {
  const style: React.CSSProperties = {
    width: typeof width === 'number' ? `${width}px` : width,
    height: typeof height === 'number' ? `${height}px` : height,
  };

  return (
    <span
      className={`skeleton skeleton-${variant} ${className}`}
      style={style}
      aria-hidden="true"
    />
  );
}

export const Skeleton = memo(SkeletonComponent);

interface SkeletonTableProps {
  rows?: number;
  columns?: number;
}

function SkeletonTableComponent({ rows = 5, columns = 6 }: SkeletonTableProps) {
  return (
    <div className="skeleton-table-wrapper" role="status" aria-label="Loading table data">
      <div className="skeleton-table">
        <div className="skeleton-table-header">
          {Array.from({ length: columns }).map((_, i) => (
            <div key={`header-${i}`} className="skeleton-table-cell">
              <Skeleton variant="text" width="80%" height={12} />
            </div>
          ))}
        </div>
        <div className="skeleton-table-body">
          {Array.from({ length: rows }).map((_, rowIndex) => (
            <div key={`row-${rowIndex}`} className="skeleton-table-row">
              {Array.from({ length: columns }).map((_, colIndex) => (
                <div key={`cell-${rowIndex}-${colIndex}`} className="skeleton-table-cell">
                  <Skeleton
                    variant="text"
                    width={colIndex === 0 ? '90%' : `${60 + Math.random() * 30}%`}
                    height={14}
                  />
                </div>
              ))}
            </div>
          ))}
        </div>
      </div>
      <span className="sr-only">Loading table data...</span>
    </div>
  );
}

export const SkeletonTable = memo(SkeletonTableComponent);

interface SkeletonCardProps {
  lines?: number;
}

function SkeletonCardComponent({ lines = 3 }: SkeletonCardProps) {
  return (
    <div className="skeleton-card" role="status" aria-label="Loading">
      <Skeleton variant="text" width="60%" height={18} className="skeleton-card-title" />
      {Array.from({ length: lines }).map((_, i) => (
        <Skeleton
          key={i}
          variant="text"
          width={i === lines - 1 ? '40%' : '100%'}
          height={14}
          className="skeleton-card-line"
        />
      ))}
      <span className="sr-only">Loading...</span>
    </div>
  );
}

export const SkeletonCard = memo(SkeletonCardComponent);
