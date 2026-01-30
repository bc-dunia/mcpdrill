import { createContext, useContext, useState, ReactNode } from 'react';
import { createI18n, Locale } from './index';

type I18nContextType = ReturnType<typeof createI18n> & {
  setLocale: (locale: Locale) => void;
};

const I18nContext = createContext<I18nContextType | null>(null);

export function I18nProvider({ children }: { children: ReactNode }) {
  const [locale, setLocale] = useState<Locale>('en');
  const i18n = createI18n(locale);
  
  return (
    <I18nContext.Provider value={{ ...i18n, setLocale }}>
      {children}
    </I18nContext.Provider>
  );
}

export function useI18n() {
  const context = useContext(I18nContext);
  if (!context) throw new Error('useI18n must be used within I18nProvider');
  return context;
}
