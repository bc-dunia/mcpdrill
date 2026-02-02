import type { TruncationInfo } from '../types';
import { Icon } from './Icon';

interface TruncationBannerProps {
  truncationInfo: TruncationInfo;
  onDismiss: () => void;
}

export function TruncationBanner({ truncationInfo, onDismiss }: TruncationBannerProps) {
  const hasAnyTruncation = truncationInfo.operationsTruncated || 
                           truncationInfo.logsTruncated || 
                           truncationInfo.dataTruncated;

  if (!hasAnyTruncation) {
    return null;
  }

  return (
    <div className="truncation-banner" role="alert">
      <div className="truncation-banner-content">
        <Icon name="alert-triangle" size="sm" aria-hidden={true} />
        <span className="truncation-banner-message">
          Data truncated: Some operations exceeded storage limits. Metrics may be incomplete.
        </span>
      </div>
      <button
        type="button"
        className="truncation-banner-dismiss"
        onClick={onDismiss}
        aria-label="Dismiss truncation warning"
      >
        <Icon name="x" size="sm" aria-hidden={true} />
      </button>
    </div>
  );
}
