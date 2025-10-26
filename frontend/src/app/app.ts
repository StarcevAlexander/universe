import {Component, OnInit} from '@angular/core';
import {HttpClient} from '@angular/common/http';
import {NgForOf, NgIf} from '@angular/common';

@Component({
  selector: 'app-root',
  imports: [NgIf, NgForOf],
  templateUrl: './app.html',
  styleUrl: './app.scss'
})
export class App implements OnInit {
  users: any[] = [];
  videos: any[] = [];
  messages = {
    email: '',
    upload: '',
    video: ''
  };

  constructor(private http: HttpClient) {
  }

  ngOnInit() {
    this.loadVideos();
  }

  // Загрузка пользователей
  loadData() {
    this.http.get<any[]>('/api/users').subscribe({
      next: (users) => {
        this.users = users;
      },
      error: (error) => {
        this.showMessage('upload', 'error', `❌ ${error.message}`);
      }
    });
  }

  // Экспорт CSV
  exportCSV() {
    window.open('/api/export-csv', '_blank');
  }

  // Импорт CSV
  uploadCSV(event: any) {
    const file = event.target.files[0];
    if (!file) return;

    const formData = new FormData();
    formData.append('file', file);

    this.showMessage('upload', 'loading', 'Загрузка...');

    this.http.post<any>('/api/upload-csv', formData).subscribe({
      next: (result) => {
        this.showMessage('upload', 'success', `✅ Успех! Загружено строк: ${result.rows || 0}`);
        this.loadData();
        event.target.value = '';
      },
      error: (error) => {
        this.showMessage('upload', 'error', `❌ Ошибка: ${error.error?.message || error.message}`);
      }
    });
  }

  // Отправка по email
  sendCSVByEmail() {
    this.showMessage('email', 'loading', 'Отправка...');

    this.http.post<any>('/api/send-csv-email', {}).subscribe({
      next: () => {
        this.showMessage('email', 'success', '✅ CSV файл успешно отправлен на почту');
      },
      error: (error) => {
        this.showMessage('email', 'error', `❌ Ошибка: ${error.error?.message || error.message}`);
      }
    });
  }

  // Загрузка видео
  uploadVideo(event: any) {
    const file = event.target.files[0];
    if (!file) return;

    if (file.size > 100 * 1024 * 1024) {
      this.showMessage('video', 'error', 'Файл слишком большой (макс. 100MB)');
      return;
    }

    const formData = new FormData();
    formData.append('video', file);

    this.showMessage('video', 'loading', 'Загрузка видео...');

    this.http.post<any>('/api/upload-video', formData).subscribe({
      next: (result) => {
        this.showMessage('video', 'success', `✅ Видео успешно загружено: ${result.filename}`);
        this.loadVideos();
        event.target.value = '';
      },
      error: (error) => {
        this.showMessage('video', 'error', `❌ Ошибка: ${error.error?.message || error.message}`);
      }
    });
  }

  // Загрузка списка видео
  loadVideos() {
    this.http.get<any[]>('/api/videos').subscribe({
      next: (videos) => {
        this.videos = videos;
      },
      error: (error) => {
        console.error('Ошибка загрузки видео:', error);
      }
    });
  }

  // Просмотр видео
  viewVideo(filename: string) {
    const videoUrl = `/api/video/${filename}`; // Используем API путь

    // Создаем попап элемент
    const popup = document.createElement('div');
    popup.className = 'video-popup';
    popup.innerHTML = `
    <div class="video-popup-overlay"></div>
    <div class="video-popup-content">
      <div class="video-popup-header">
        <h3>${filename}</h3>
        <button class="close-btn">×</button>
      </div>
      <div class="video-container">
        <video controls autoplay preload="metadata" playsinline>
          <source src="${videoUrl}" type="video/mp4">
          Ваш браузер не поддерживает видео тег.
        </video>
        <div class="video-loading">Загрузка видео...</div>
        <div class="video-error" style="display: none;">
          Ошибка загрузки видео. Попробуйте позже.
        </div>
      </div>
      <div class="video-info">
        <div class="video-stats">
          <span>📊 Загрузка: <span class="load-status">0%</span></span>
          <span>⏱️ Длительность: <span class="duration-status">--:--</span></span>
        </div>
      </div>
    </div>
  `;

    // Добавляем попап на страницу
    document.body.appendChild(popup);
    document.body.style.overflow = 'hidden'; // Блокируем скролл страницы

    const video = popup.querySelector('video') as HTMLVideoElement;
    const closeBtn = popup.querySelector('.close-btn') as HTMLButtonElement;
    const overlay = popup.querySelector('.video-popup-overlay') as HTMLDivElement;
    const loadingEl = popup.querySelector('.video-loading') as HTMLDivElement;
    const errorEl = popup.querySelector('.video-error') as HTMLDivElement;
    const loadStatus = popup.querySelector('.load-status') as HTMLSpanElement;
    const durationStatus = popup.querySelector('.duration-status') as HTMLSpanElement;

    // Обработчики событий видео
    if (video) {
      video.addEventListener('loadstart', () => {
        loadingEl.style.display = 'block';
      });

      video.addEventListener('loadeddata', () => {
        loadingEl.style.display = 'none';
      });

      video.addEventListener('progress', () => {
        if (video.buffered.length > 0) {
          const bufferedEnd = video.buffered.end(video.buffered.length - 1);
          const duration = video.duration;
          if (duration > 0) {
            const percent = (bufferedEnd / duration * 100).toFixed(1);
            loadStatus.textContent = `${percent}%`;
          }
        }
      });

      video.addEventListener('loadedmetadata', () => {
        const duration = video.duration;
        if (duration && isFinite(duration)) {
          const minutes = Math.floor(duration / 60);
          const seconds = Math.floor(duration % 60);
          durationStatus.textContent = `${minutes}:${seconds.toString().padStart(2, '0')}`;
        }
      });

      video.addEventListener('error', () => {
        loadingEl.style.display = 'none';
        errorEl.style.display = 'block';
        console.error('Ошибка загрузки видео:', video.error);
      });

      video.addEventListener('canplaythrough', () => {
        loadingEl.style.display = 'none';
      });

      // Автоматический фокус на видео для управления с клавиатуры
      setTimeout(() => video.focus(), 100);
    }

    // Функция закрытия попапа
    const closePopup = () => {
      if (video) {
        video.pause();
        video.src = ''; // Освобождаем ресурсы
      }
      popup.remove();
      document.body.style.overflow = '';
      document.removeEventListener('keydown', closeOnEsc);
    };

    // Закрытие по клику на кнопку
    closeBtn.addEventListener('click', closePopup);

    // Закрытие по клику на оверлей
    overlay.addEventListener('click', closePopup);

    // Закрытие по ESC
    const closeOnEsc = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        closePopup();
      }
    };
    document.addEventListener('keydown', closeOnEsc);

    // Предотвращаем закрытие при клике на само видео
    const videoContent = popup.querySelector('.video-popup-content') as HTMLDivElement;
    videoContent.addEventListener('click', (event) => {
      event.stopPropagation();
    });
  }

  // Удаление видео
  deleteVideo(filename: string) {
    if (!confirm(`Удалить видео "${filename}"?`)) return;

    this.http.delete(`/api/delete-video/${filename}`).subscribe({
      next: () => {
        alert('✅ Видео успешно удалено');
        this.loadVideos();
      },
      error: (error) => {
        alert(`❌ Ошибка: ${error.error?.message || error.message}`);
      }
    });
  }

  // Вспомогательная функция для сообщений
  private showMessage(type: 'email' | 'upload' | 'video', status: 'success' | 'error' | 'loading', text: string) {
    this.messages[type] = text;

    // Автоочистка сообщений через 5 секунд
    if (status !== 'loading') {
      setTimeout(() => {
        this.messages[type] = '';
      }, 5000);
    }
  }
}
