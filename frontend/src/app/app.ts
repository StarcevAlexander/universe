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

    if (file.size > 2 * 1024 * 1024 * 1024) {
      this.showMessage('video', 'error', 'Файл слишком большой (макс. 2GB)');
      return;
    }

    const formData = new FormData();
    formData.append('video', file);

    this.http.post<any>('/api/upload-video', formData).subscribe({
      next: (result) => {
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
    const videoUrl = `/api/video/${filename}`;

    // Блокируем скролл body
    document.body.classList.add('popup-open');

    const popup = document.createElement('div');
    popup.className = 'video-popup';
    popup.innerHTML = `
    <div class="video-popup-overlay"></div>
    <div class="video-popup-content">
      <div class="video-popup-header">
        <h3>${filename}</h3>
        <button class="close-btn" aria-label="Закрыть видео">×</button>
      </div>
      <div class="video-container">
        <video controls autoplay preload="metadata" playsinline>
          <source src="${videoUrl}" type="video/mp4">
          Ваш браузер не поддерживает видео тег.
        </video>
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

    document.body.appendChild(popup);

    const closePopup = () => {
      const video = popup.querySelector('video') as HTMLVideoElement;
      if (video) {
        video.pause();
        video.src = '';
      }
      popup.remove();
      document.body.classList.remove('popup-open');
      document.removeEventListener('keydown', closeOnEsc);
    };

    const closeBtn = popup.querySelector('.close-btn') as HTMLButtonElement;
    const overlay = popup.querySelector('.video-popup-overlay') as HTMLDivElement;

    closeBtn.addEventListener('click', closePopup);
    overlay.addEventListener('click', closePopup);

    const closeOnEsc = (event: KeyboardEvent) => {
      if (event.key === 'Escape') closePopup();
    };
    document.addEventListener('keydown', closeOnEsc);

    // Предотвращаем закрытие при клике на контент
    const content = popup.querySelector('.video-popup-content') as HTMLDivElement;
    content.addEventListener('click', (event) => event.stopPropagation());
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
