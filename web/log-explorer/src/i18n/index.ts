export type Locale = 'en' | 'ko';

const translations = {
  en: {
    // Navigation
    'nav.logs': 'Log Explorer',
    'nav.newRun': 'New Run…',
    'nav.compare': 'Compare Runs',
    // Common
    'common.loading': 'Loading...',
    'common.error': 'Error',
    'common.retry': 'Try Again',
    'common.clear': 'Clear',
    'common.cancel': 'Cancel',
    'common.confirm': 'Confirm',
    'common.start': 'Start',
    'common.export': 'Export',
    'common.search': 'Search...',
    'common.noResults': 'No results found',
    // Log Explorer
    'logs.title': 'Log Explorer',
    'logs.selectRun': 'Select a run...',
    'logs.noLogs': 'No logs found',
    'logs.filters': 'Filters',
    // Wizard
    'wizard.step1': 'Target',
    'wizard.step2': 'Stages',
    'wizard.step3': 'Workload',
    'wizard.step4': 'Review',
    // Glossary (for onboarding)
    'glossary.vu': 'Virtual User - A simulated client making requests',
    'glossary.stage': 'A phase of the load test with specific settings',
    'glossary.ramp': 'Gradually increasing load to find limits',
    'glossary.preflight': 'Quick connectivity check before full test',
    'glossary.baseline': 'Steady load to establish normal metrics',
  },
  ko: {
    // Navigation
    'nav.logs': '로그 탐색기',
    'nav.newRun': '새 실행',
    'nav.compare': '실행 비교',
    // Common
    'common.loading': '로딩 중...',
    'common.error': '오류',
    'common.retry': '다시 시도',
    'common.clear': '지우기',
    'common.cancel': '취소',
    'common.confirm': '확인',
    'common.start': '시작',
    'common.export': '내보내기',
    'common.search': '검색...',
    'common.noResults': '결과가 없습니다',
    // Log Explorer
    'logs.title': '로그 탐색기',
    'logs.selectRun': '실행 선택...',
    'logs.noLogs': '로그가 없습니다',
    'logs.filters': '필터',
    // Wizard
    'wizard.step1': '대상',
    'wizard.step2': '단계',
    'wizard.step3': '워크로드',
    'wizard.step4': '검토',
    // Glossary (for onboarding)
    'glossary.vu': '가상 사용자 - 요청을 보내는 시뮬레이션 클라이언트',
    'glossary.stage': '특정 설정이 있는 부하 테스트 단계',
    'glossary.ramp': '한계를 찾기 위해 부하를 점진적으로 증가',
    'glossary.preflight': '전체 테스트 전 빠른 연결 확인',
    'glossary.baseline': '정상 메트릭을 설정하기 위한 안정적인 부하',
  }
};

export type TranslationKey = keyof typeof translations['en'];

export function createI18n(locale: Locale = 'en') {
  return {
    t: (key: string): string => {
      return translations[locale][key as TranslationKey] || key;
    },
    locale,
  };
}
