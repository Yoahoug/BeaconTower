/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{vue,js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        toy: {
          bg: '#FAF7EE',
          card: '#FFFFFF',
          black: '#121212',
          yellow: '#FFE047',
          'yellow-light': '#FFF3A3',
          pink: '#FF5E85',
          'pink-light': '#FFE3EB',
          mint: '#00E599',
          'mint-light': '#D6FBEF',
          cyan: '#38BDF8',
          'cyan-light': '#E0F7FE',
          orange: '#FB923C',
          purple: '#A78BFA',
          'purple-light': '#EDE9FE',
          gray: '#E4E4E7',
          muted: '#71717A',
        }
      },
      boxShadow: {
        'toy': '4px 4px 0px #121212',
        'toy-sm': '2px 2px 0px #121212',
        'toy-lg': '6px 6px 0px #121212',
        'toy-inset': 'inset 2px 2px 0px #121212',
      },
      borderWidth: {
        '2.5': '2.5px',
        '3': '3px',
      }
    },
  },
  plugins: [],
}
